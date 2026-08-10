"""Shared configuration types for MCP servers that connect to SAP systems."""

import json
import os
import re
from pathlib import Path
from typing import Annotated, Any, Literal

import yaml
from dotenv import load_dotenv
from pydantic import BaseModel, BeforeValidator, ConfigDict, SecretStr, model_validator

#: Default config file path when SAP_CONFIG_FILE is not set.
DEFAULT_CONFIG_PATH = "~/.config/sap-mcp/systems.json"

#: Language field type that normalizes to uppercase before validation.
Language = Annotated[Literal["DE", "EN"], BeforeValidator(lambda v: v.upper() if isinstance(v, str) else v)]

#: Matches an ``${env:VAR}`` placeholder.  ``VAR`` must be a plain identifier,
#: so anything else — e.g. ``${env:not an identifier}`` — stays a literal and
#: can never be mistaken for a placeholder.
_ENV_PLACEHOLDER = re.compile(r"\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}")

#: Fields whose value may carry an ``${env:VAR}`` placeholder.  ``language`` is
#: deliberately absent: it is a ``Literal`` and pydantic already rejects an
#: unresolved placeholder during field validation, before the model validator
#: below ever runs.
_INTERPOLATED_FIELDS = ("connection_name", "host", "client", "user", "password", "oauth2_client_id")


def _interpolate_env(value: Any) -> Any:
    """Replace ``${env:VAR}`` with the environment value in every string of a parsed document.

    Substitution is **single-pass**: replacement text is never rescanned, so a
    secret whose value happens to contain ``${env:...}`` cannot be used to read
    another variable.

    Placeholders whose variable is unset are left verbatim;
    :meth:`Config._validate` turns those into a collected configuration error
    rather than silently yielding an empty string.
    """
    if isinstance(value, str):
        return _ENV_PLACEHOLDER.sub(lambda m: os.environ.get(m.group(1), m.group(0)), value)
    if isinstance(value, dict):
        return {key: _interpolate_env(item) for key, item in value.items()}
    if isinstance(value, list):
        return [_interpolate_env(item) for item in value]
    return value


def _unresolved_env_var(value: str) -> str | None:
    """Return the name of the first ``${env:VAR}`` in *value* whose variable is unset.

    Only a genuinely unset variable counts.  Because interpolation is
    single-pass, a placeholder that survives into a *resolved* value must have
    come from the environment itself — and if that variable is set, the text is
    a literal the user meant to keep, not a mistake.
    """
    for match in _ENV_PLACEHOLDER.finditer(value):
        if match.group(1) not in os.environ:
            return match.group(1)
    return None


def _collect_unresolved(name: str, system: "SAPSystem") -> tuple[list[str], set[str]]:
    """Report every field of *system* still holding a placeholder.

    Returns the error messages and the names of the affected fields, so the
    caller can skip their remaining checks.  Only the variable *name* is
    reported — never the value it resolved to — so a validation failure can't
    echo a credential.
    """
    errs: list[str] = []
    unresolved: set[str] = set()
    for field in _INTERPOLATED_FIELDS:
        raw = getattr(system, field)
        text = raw.get_secret_value() if isinstance(raw, SecretStr) else raw
        var = _unresolved_env_var(text)
        if var is not None:
            unresolved.add(field)
            errs.append(f'system "{name}": {field} references ${{env:{var}}}, which is not set in the environment')
    return errs, unresolved


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
    def _validate(self) -> "Config":
        """Collect **all** validation errors so users can fix everything in one pass."""
        errs: list[str] = []
        if not self.systems:
            raise ValueError("config has no systems defined")
        unresolved_default = _unresolved_env_var(self.default_system)
        if unresolved_default is not None:
            errs.append(f"default_system references ${{env:{unresolved_default}}}, which is not set in the environment")
        elif self.default_system not in self.systems:
            errs.append(f'default_system "{self.default_system}" not found in systems')
        for name, sys in self.systems.items():
            # A field still holding a placeholder has no meaningful value yet, so
            # its remaining checks are skipped — reporting 'host must start with
            # http://, got "${env:SAP_HOST}"' on top of the unset-variable error
            # would just be noise. Crucially this also suppresses the both-or-
            # neither check, which would otherwise read an unresolved user and
            # password as a deliberate OAuth2 system.
            unresolved_errs, unresolved = _collect_unresolved(name, sys)
            errs.extend(unresolved_errs)
            if "host" not in unresolved:
                if not sys.host:
                    errs.append(f'system "{name}": host is required')
                elif not sys.host.startswith(("http://", "https://")):
                    errs.append(f'system "{name}": host must start with http:// or https://, got "{sys.host}"')
            if "client" not in unresolved and sys.client and (len(sys.client) != 3 or not sys.client.isdigit()):
                errs.append(f'system "{name}": client must be a 3-digit string (e.g. "100"), got "{sys.client}"')
            pwd = sys.password.get_secret_value()
            if not unresolved & {"user", "password"} and (sys.user == "") != (pwd == ""):
                errs.append(f'system "{name}": must have both user and password, or neither (for OAuth2)')
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
    raw = _interpolate_env(json.loads(data))
    return Config(**raw)


def parse_yaml(data: str | bytes) -> Config:
    """Parse a YAML string or bytes into a validated Config.

    Raises ``pydantic.ValidationError`` with human-readable messages
    if the configuration is invalid.
    """
    raw = yaml.safe_load(data)
    if not isinstance(raw, dict):
        raise ValueError("expected a YAML mapping at the top level")
    return Config(**_interpolate_env(raw))


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
