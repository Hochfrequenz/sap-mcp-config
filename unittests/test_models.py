"""Tests for sap_mcp_config — must stay consistent with config_test.go."""

from pathlib import Path

import pytest
from pydantic import ValidationError

from sap_mcp_config import Config, SAPSystem, load, parse

TESTDATA = Path(__file__).resolve().parent.parent / "testdata" / "systems.json"


class TestLoadTestFixture:
    """Parse the shared testdata/systems.json — same assertions as the Go tests."""

    def test_load_fixture(self) -> None:
        cfg = load(TESTDATA)

        assert cfg.default_system == "dev"
        assert len(cfg.systems) == 3

        dev = cfg.systems["dev"]
        assert dev.host == "https://dev-sap.example.com:44300"
        assert dev.client == "100"
        assert dev.user == "DEV_USER"
        assert dev.password == "dev_secret"
        assert dev.language == "DE"
        assert dev.tls_skip_verify is True
        assert dev.is_oauth2 is False

        prod = cfg.systems["prod"]
        assert prod.host == "https://prod-sap.example.com:44300"
        assert prod.client == "200"
        assert prod.user == "PROD_USER"
        assert prod.password == "prod_secret"
        assert prod.language == "EN"
        assert prod.tls_skip_verify is False
        assert prod.is_oauth2 is False

        oauth = cfg.systems["oauth"]
        assert oauth.host == "https://oauth-sap.example.com:44300"
        assert oauth.client == "300"
        assert oauth.user == ""
        assert oauth.password == ""
        assert oauth.language == "EN"
        assert oauth.oauth2_client_id == "my-mcp-client"
        assert oauth.is_oauth2 is True


TESTDATA_SPECIAL = Path(__file__).resolve().parent.parent / "testdata" / "special_characters.json"


class TestSpecialCharacterPasswords:
    """Passwords with quotes, backslashes, unicode — same assertions as Go tests."""

    def test_special_characters(self) -> None:
        cfg = load(TESTDATA_SPECIAL)
        assert len(cfg.systems) == 3

        tricky = cfg.systems["tricky"]
        assert tricky.password == 'p@ss"w0rd\'with<special>&chars!{}[]'

        unicode_sys = cfg.systems["unicode"]
        assert unicode_sys.user == "UMLAUT_ÜÖÄ"
        assert unicode_sys.password == "äöüß€£"

        backslash = cfg.systems["backslash"]
        assert backslash.user == "DOMAIN\\USER"
        assert backslash.password == "pass\\word\\with\\backslashes"


class TestParseValidation:
    def test_no_systems(self) -> None:
        with pytest.raises(ValidationError, match="no systems defined"):
            parse('{"default_system":"x","systems":{}}')

    def test_default_not_found(self) -> None:
        with pytest.raises(ValidationError, match='default_system "missing" not found'):
            parse('{"default_system":"missing","systems":{"a":{"host":"h","user":"u","password":"p"}}}')

    def test_user_without_password(self) -> None:
        with pytest.raises(ValidationError, match="must have both user and password"):
            parse('{"default_system":"a","systems":{"a":{"host":"h","user":"u"}}}')

    def test_password_without_user(self) -> None:
        with pytest.raises(ValidationError, match="must have both user and password"):
            parse('{"default_system":"a","systems":{"a":{"host":"h","password":"p"}}}')


class TestParseMinimal:
    def test_minimal(self) -> None:
        data = '{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}'
        cfg = parse(data)
        assert len(cfg.systems) == 1


class TestLoadFileNotFound:
    def test_file_not_found(self) -> None:
        with pytest.raises(FileNotFoundError):
            load("nonexistent.json")
