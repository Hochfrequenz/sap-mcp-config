"""Shared configuration types for MCP servers that connect to SAP systems."""

import json
import os
import re
from pathlib import Path
from typing import Annotated, Any

import yaml
from dotenv import load_dotenv
from pydantic import BaseModel, BeforeValidator, ConfigDict, SecretStr, ValidationInfo, model_validator

#: Default config file path when SAP_CONFIG_FILE is not set.
DEFAULT_CONFIG_PATH = "~/.config/sap-mcp/systems.json"


def _normalize_language(value: object) -> object:
    """Uppercase a language and treat an empty one as the default."""
    if isinstance(value, str):
        return value.upper() or "EN"
    return value


#: Language field type that normalizes to uppercase before validation.
#:
#: The allowed values are checked in :meth:`Config._validate` rather than by a
#: ``Literal``: a field-level failure aborts validation before the model
#: validator runs, which would drop every other collected error and disagree
#: with Go, where an invalid language is just one more entry in the list.
Language = Annotated[str, BeforeValidator(_normalize_language)]

#: Matches an ``${env:VAR}`` placeholder.  ``VAR`` must be a plain identifier,
#: so anything else — e.g. ``${env:not an identifier}`` — stays a literal and
#: can never be mistaken for a placeholder.
#:
#: Deliberately **not** anchored with ``^``/``$``: a placeholder may sit anywhere
#: inside a larger value, and a value may hold several — ``"https://${env:HOST}:
#: ${env:PORT}"`` must resolve both.  Anchoring would restrict placeholders to
#: whole-value use only and silently break that.  The identifier character class
#: is what keeps matches tight, not the position.
_ENV_PLACEHOLDER = re.compile(r"\$\{env:(?P<var>[A-Za-z_][A-Za-z0-9_]*)\}")

#: Fields whose value may carry an ``${env:VAR}`` placeholder — the string
#: fields of a system.  Kept identical to the list in ``config.go``.
_INTERPOLATED_FIELDS = ("connection_name", "host", "client", "user", "password", "language", "oauth2_client_id")

#: Key under which the :class:`_PlaceholderReport` is passed to the validator.
_REPORT_CONTEXT_KEY = "sap_mcp_config.placeholder_report"


class _FieldStatus(BaseModel):
    """What interpolation did to a single field."""

    model_config = ConfigDict(frozen=True)

    #: True when any part of the value came from a placeholder.
    from_env: bool = False
    #: True when a placeholder could not be turned into a usable value, so the
    #: field has no meaningful content to validate.
    unusable: bool = False


class _PlaceholderReport(BaseModel):
    """The outcome of interpolating one config document.

    Carries the errors to report plus per-field status, so :meth:`Config._validate`
    can skip fields that have no usable value and avoid echoing values that came
    from the environment.

    :attr:`status` is keyed by system name and then field name; the empty system
    name holds the top-level ``default_system`` entry.
    """

    messages: list[str] = []
    status: dict[str, dict[str, _FieldStatus]] = {}

    def get(self, system: str, field: str) -> _FieldStatus:
        """Return the status recorded for *field*, or a neutral one."""
        return self.status.get(system, {}).get(field, _FieldStatus())

    def unusable(self, system: str, field: str) -> bool:
        """True when *field* has no usable value because a placeholder failed to resolve."""
        return self.get(system, field).unusable

    def describe(self, system: str, field: str, value: str) -> str:
        """Render *value* for an error message, withholding it when it came from the environment.

        An env-supplied host or client may itself be sensitive, and the user did
        not write it in the file, so echoing it back helps nobody.
        """
        if self.get(system, field).from_env:
            return "the value taken from the environment"
        return f'"{value}"'

    def _add(self, system: str, field: str, detail: str) -> None:
        if system == "":
            self.messages.append(f"{field} {detail}")
        else:
            self.messages.append(f'system "{system}": {field} {detail}')

    def resolve(self, system: str, field: str, value: str) -> str:
        """Replace every ``${env:VAR}`` in *value* and record what happened.

        Substitution is **single-pass**: replacement text is never rescanned, so
        a secret whose value happens to contain ``${env:...}`` cannot be used to
        read another variable.

        Detection happens here rather than by re-scanning the finished value,
        because only at this point do we still know the placeholder came from the
        config document.  Re-scanning afterwards would misread a secret that
        legitimately contains ``${env:...}`` as an unresolved placeholder — and
        would build the error message out of that secret's plaintext.
        """
        from_env = False
        unusable = False

        def _substitute(match: re.Match[str]) -> str:
            nonlocal from_env, unusable
            name = match.group("var")
            from_env = True
            if name not in os.environ:
                unusable = True
                self._add(system, field, f"references ${{env:{name}}}, which is not set in the environment")
                return match.group(0)
            env_value = os.environ[name]
            if env_value == "":
                # Defined-but-empty is the shape an unpopulated CI secret takes,
                # and letting it through would silently strip a credential — for
                # user and password that even flips the system to OAuth2.
                unusable = True
                self._add(system, field, f"references ${{env:{name}}}, which is set but empty")
            return env_value

        resolved = _ENV_PLACEHOLDER.sub(_substitute, value)
        if from_env:
            self.status.setdefault(system, {})[field] = _FieldStatus(from_env=from_env, unusable=unusable)
        return resolved


def _interpolate(raw: dict[str, Any]) -> _PlaceholderReport:
    """Resolve ``${env:VAR}`` in the known string fields of *raw*, in place.

    Only the documented fields are visited, rather than every string in the
    document.  That keeps Python and Go in step — Go interpolates the decoded
    struct, which has no way to reach an arbitrary node — and it means a
    self-referential YAML document (``x: &a [*a]``) cannot send this into
    unbounded recursion.
    """
    report = _PlaceholderReport()
    default_system = raw.get("default_system")
    if isinstance(default_system, str):
        raw["default_system"] = report.resolve("", "default_system", default_system)
    systems = raw.get("systems")
    if isinstance(systems, dict):
        for name, system in systems.items():
            if not isinstance(name, str) or not isinstance(system, dict):
                continue  # malformed; let pydantic report it
            for field in _INTERPOLATED_FIELDS:
                value = system.get(field)
                if isinstance(value, str):
                    system[field] = report.resolve(name, field, value)
    return report


class SAPSystem(BaseModel):
    """A single SAP system's connection details and credentials.

    The ``connection_name`` field stores the SAP Logon connection entry name
    (the bold description text in the SAP Logon pad).  It is independent of
    the dictionary key under which this system is stored in :class:`Config`.

    Either both ``user`` and ``password`` must be set, or neither (for OAuth2).

    The ``password`` field is a :class:`~pydantic.SecretStr` so that it is
    never accidentally printed or logged.  Access the plain text value via
    ``system.password.get_secret_value()``.
    """

    model_config = ConfigDict(frozen=True)

    connection_name: str = ""
    host: str = ""
    client: Annotated[str, BeforeValidator(lambda v: str(v) if isinstance(v, (int, float)) else v)] = ""
    user: str = ""
    password: SecretStr = SecretStr("")
    language: Language = "EN"
    tls_skip_verify: bool = False
    oauth2_client_id: str = ""

    @property
    def is_oauth2(self) -> bool:
        """True when the system is configured for OAuth2 (no user/password)."""
        return self.user == "" and self.password.get_secret_value() == ""


class Config(BaseModel):
    """All configured SAP systems and a default system name.

    Use :func:`load`, :func:`load_default`, or :func:`parse` to create
    instances — they validate the configuration before returning it.
    """

    model_config = ConfigDict(frozen=True)

    default_system: str
    systems: dict[str, SAPSystem]

    @model_validator(mode="after")
    def _validate(self, info: ValidationInfo) -> "Config":
        """Collect **all** validation errors so users can fix everything in one pass."""
        context = info.context if isinstance(info.context, dict) else {}
        # A directly constructed Config carries no report, which simply means no
        # interpolation happened — the same as Go's exported Validate.
        report = context.get(_REPORT_CONTEXT_KEY) or _PlaceholderReport()
        # Placeholder problems are reported alongside everything else so the user
        # still fixes the whole file in one pass.
        errs: list[str] = list(report.messages)
        if not self.systems:
            raise ValueError("config has no systems defined")
        if not report.unusable("", "default_system") and self.default_system not in self.systems:
            errs.append(f'default_system "{self.default_system}" not found in systems')
        for name, sys in self.systems.items():
            # A field whose placeholder could not be resolved has no meaningful
            # value, so its remaining checks are skipped — reporting 'host must
            # start with http://' on top of the unset-variable error would just be
            # noise. Crucially this also suppresses the both-or-neither check,
            # which would otherwise read an unresolved user and password as a
            # deliberate OAuth2 system.
            if not report.unusable(name, "host"):
                if not sys.host:
                    errs.append(f'system "{name}": host is required')
                elif not sys.host.startswith(("http://", "https://")):
                    described = report.describe(name, "host", sys.host)
                    errs.append(f'system "{name}": host must start with http:// or https://, got {described}')
            if (
                not report.unusable(name, "client")
                and sys.client
                and (len(sys.client) != 3 or not sys.client.isdigit())
            ):
                described = report.describe(name, "client", sys.client)
                errs.append(f'system "{name}": client must be a 3-digit string (e.g. "100"), got {described}')
            pwd = sys.password.get_secret_value()
            if not (report.unusable(name, "user") or report.unusable(name, "password")) and (sys.user == "") != (
                pwd == ""
            ):
                errs.append(f'system "{name}": must have both user and password, or neither (for OAuth2)')
            if not report.unusable(name, "language") and sys.language not in ("DE", "EN"):
                described = report.describe(name, "language", sys.language)
                errs.append(f'system "{name}": language must be "DE" or "EN", got {described}')
        if errs:
            raise ValueError("invalid configuration:\n  - " + "\n  - ".join(errs))
        return self

    def get_default(self) -> SAPSystem:
        """Return the default system's configuration."""
        return self.systems[self.default_system]


def parse(data: str | bytes) -> Config:
    """Parse a JSON string or bytes into a validated Config.

    Raises ``pydantic.ValidationError`` with human-readable messages
    if the configuration is invalid.
    """
    raw = json.loads(data)
    if not isinstance(raw, dict):
        raise ValueError("expected a JSON object at the top level")
    report = _interpolate(raw)
    return Config.model_validate(raw, context={_REPORT_CONTEXT_KEY: report})


def parse_yaml(data: str | bytes) -> Config:
    """Parse a YAML string or bytes into a validated Config.

    Raises ``pydantic.ValidationError`` with human-readable messages
    if the configuration is invalid.
    """
    raw = yaml.safe_load(data)
    if not isinstance(raw, dict):
        raise ValueError("expected a YAML mapping at the top level")
    report = _interpolate(raw)
    return Config.model_validate(raw, context={_REPORT_CONTEXT_KEY: report})


_YAML_EXTENSIONS = {".yaml", ".yml"}


def load(path: str | Path) -> Config:
    """Load a Config from a JSON or YAML file.

    The format is detected by file extension: ``.yaml`` / ``.yml`` for YAML,
    everything else (including ``.json``) for JSON.

    The *path* may start with ``~`` which is expanded to the user's home
    directory.
    """
    resolved = Path(path).expanduser()
    data = resolved.read_bytes()
    if resolved.suffix.lower() in _YAML_EXTENSIONS:
        return parse_yaml(data)
    return parse(data)


def load_default() -> Config:
    """Load configuration from ``SAP_CONFIG_FILE`` env var, falling back to
    :data:`DEFAULT_CONFIG_PATH`.

    Loads ``.env`` files from the current directory before reading the
    environment variable.
    """
    load_dotenv()  # best-effort; missing .env is fine
    path = os.environ.get("SAP_CONFIG_FILE", DEFAULT_CONFIG_PATH)
    return load(path)
