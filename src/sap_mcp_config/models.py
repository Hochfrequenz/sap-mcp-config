"""Shared configuration types for MCP servers that connect to SAP systems."""

import json
import os
from pathlib import Path
from typing import Annotated, Literal

import yaml
from dotenv import load_dotenv
from pydantic import BaseModel, BeforeValidator, ConfigDict, SecretStr, model_validator

#: Default config file path when SAP_CONFIG_FILE is not set.
DEFAULT_CONFIG_PATH = "~/.config/sap-mcp/systems.json"

#: Language field type that normalizes to uppercase before validation.
Language = Annotated[Literal["DE", "EN"], BeforeValidator(lambda v: v.upper() if isinstance(v, str) else v)]


class SAPSystem(BaseModel):
    """A single SAP system's connection details and credentials.

    Either both ``user`` and ``password`` must be set, or neither (for OAuth2).

    The ``password`` field is a :class:`~pydantic.SecretStr` so that it is
    never accidentally printed or logged.  Access the plain text value via
    ``system.password.get_secret_value()``.
    """

    model_config = ConfigDict(frozen=True)

    host: str = ""
    client: str = ""
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
        if self.default_system not in self.systems:
            errs.append(f'default_system "{self.default_system}" not found in systems')
        for name, sys in self.systems.items():
            if not sys.host:
                errs.append(f'system "{name}": host is required')
            elif not sys.host.startswith(("http://", "https://")):
                errs.append(f'system "{name}": host must start with http:// or https://, got "{sys.host}"')
            if sys.client and (len(sys.client) != 3 or not sys.client.isdigit()):
                errs.append(f'system "{name}": client must be a 3-digit string (e.g. "100"), got "{sys.client}"')
            pwd = sys.password.get_secret_value()
            if (sys.user == "") != (pwd == ""):
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
    raw = json.loads(data)
    return Config(**raw)


def parse_yaml(data: str | bytes) -> Config:
    """Parse a YAML string or bytes into a validated Config.

    Raises ``pydantic.ValidationError`` with human-readable messages
    if the configuration is invalid.
    """
    raw = yaml.safe_load(data)
    if not isinstance(raw, dict):
        raise ValueError("expected a YAML mapping at the top level")
    return Config(**raw)


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
