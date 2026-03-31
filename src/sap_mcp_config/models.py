"""Shared configuration types for MCP servers that connect to SAP systems."""

import json
from pathlib import Path

from pydantic import BaseModel, model_validator


class SAPSystem(BaseModel):
    """A single SAP system's connection details and credentials."""

    host: str
    client: str = ""
    user: str = ""
    password: str = ""
    language: str = "EN"
    tls_skip_verify: bool = False
    oauth2_client_id: str = ""

    @property
    def is_oauth2(self) -> bool:
        """True when the system is configured for OAuth2 (no user/password)."""
        return self.user == "" and self.password == ""

    @model_validator(mode="after")
    def _user_password_both_or_neither(self) -> "SAPSystem":
        if (self.user == "") != (self.password == ""):
            raise ValueError("must have both user and password, or neither (for OAuth2)")
        return self


class Config(BaseModel):
    """All configured SAP systems and a default system name."""

    default_system: str
    systems: dict[str, SAPSystem]

    @model_validator(mode="after")
    def _validate(self) -> "Config":
        if not self.systems:
            raise ValueError("config has no systems defined")
        if self.default_system not in self.systems:
            raise ValueError(f'default_system "{self.default_system}" not found in systems')
        return self


def parse(data: str | bytes) -> Config:
    """Parse a JSON string or bytes into a Config."""
    raw = json.loads(data)
    return Config(**raw)


def load(path: str | Path) -> Config:
    """Load a Config from a JSON file."""
    return parse(Path(path).read_bytes())
