"""Tests for sap_mcp_config — must stay consistent with config_test.go."""

from pathlib import Path

import pytest
from pydantic import ValidationError

from sap_mcp_config import SAPSystem, load, load_default, parse, parse_yaml

TESTDATA = Path(__file__).resolve().parent.parent / "testdata" / "systems.json"
TESTDATA_YAML = Path(__file__).resolve().parent.parent / "testdata" / "systems.yaml"
TESTDATA_SPECIAL = Path(__file__).resolve().parent.parent / "testdata" / "special_characters.json"


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
        assert dev.password.get_secret_value() == "dev_secret"
        assert dev.language == "DE"
        assert dev.tls_skip_verify is True
        assert dev.is_oauth2 is False

        prod = cfg.systems["prod"]
        assert prod.host == "https://prod-sap.example.com:44300"
        assert prod.client == "200"
        assert prod.user == "PROD_USER"
        assert prod.password.get_secret_value() == "prod_secret"
        assert prod.language == "EN"
        assert prod.tls_skip_verify is False
        assert prod.is_oauth2 is False

        oauth = cfg.systems["oauth"]
        assert oauth.host == "https://oauth-sap.example.com:44300"
        assert oauth.client == "300"
        assert oauth.user == ""
        assert oauth.password.get_secret_value() == ""
        assert oauth.language == "EN"
        assert oauth.oauth2_client_id == "my-mcp-client"
        assert oauth.is_oauth2 is True


class TestGetDefault:
    def test_get_default(self) -> None:
        cfg = load(TESTDATA)
        default = cfg.get_default()
        assert default.host == "https://dev-sap.example.com:44300"


class TestLanguageDefaultsToEN:
    def test_language_defaults_to_en(self) -> None:
        data = (
            '{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}'
        )
        cfg = parse(data)
        assert cfg.systems["s"].language == "EN"


class TestLanguageCaseInsensitive:
    def test_lowercase_language_normalized(self) -> None:
        data = (
            '{"default_system":"s","systems":{"s":'
            '{"host":"https://x:443","client":"100","user":"u","password":"p","language":"de"}}}'
        )
        cfg = parse(data)
        assert cfg.systems["s"].language == "DE"


class TestSpecialCharacterPasswords:
    """Passwords with quotes, backslashes, unicode — same assertions as Go tests."""

    def test_special_characters(self) -> None:
        cfg = load(TESTDATA_SPECIAL)
        assert len(cfg.systems) == 3

        tricky = cfg.systems["tricky"]
        assert tricky.password.get_secret_value() == "p@ss\"w0rd'with<special>&chars!{}[]"

        unicode_sys = cfg.systems["unicode"]
        assert unicode_sys.user == "UMLAUT_\u00dc\u00d6\u00c4"
        assert unicode_sys.password.get_secret_value() == "\u00e4\u00f6\u00fc\u00df\u20ac\u00a3"

        backslash = cfg.systems["backslash"]
        assert backslash.user == "DOMAIN\\USER"
        assert backslash.password.get_secret_value() == "pass\\word\\with\\backslashes"


class TestPasswordMasking:
    """Passwords must never leak through str/repr."""

    def test_password_masked_in_str(self) -> None:
        cfg = load(TESTDATA)
        text = str(cfg.systems["dev"])
        assert "dev_secret" not in text
        assert "**" in text  # SecretStr shows '**********'

    def test_password_masked_in_repr(self) -> None:
        cfg = load(TESTDATA)
        text = repr(cfg.systems["dev"])
        assert "dev_secret" not in text

    def test_password_accessible_via_get_secret_value(self) -> None:
        cfg = load(TESTDATA)
        assert cfg.systems["dev"].password.get_secret_value() == "dev_secret"


class TestParseValidation:
    def test_no_systems(self) -> None:
        with pytest.raises(ValidationError, match="no systems defined"):
            parse('{"default_system":"x","systems":{}}')

    def test_default_not_found(self) -> None:
        with pytest.raises(ValidationError, match='default_system "missing" not found'):
            parse('{"default_system":"missing","systems":{"a":{"host":"https://h","user":"u","password":"p"}}}')

    def test_missing_host(self) -> None:
        with pytest.raises(ValidationError, match="host"):
            parse('{"default_system":"a","systems":{"a":{"user":"u","password":"p"}}}')

    def test_invalid_host_scheme(self) -> None:
        with pytest.raises(ValidationError, match="host must start with http"):
            parse('{"default_system":"a","systems":{"a":{"host":"ftp://h","client":"100","user":"u","password":"p"}}}')

    def test_invalid_client(self) -> None:
        with pytest.raises(ValidationError, match="client must be a 3-digit string"):
            parse('{"default_system":"a","systems":{"a":{"host":"https://h","client":"1","user":"u","password":"p"}}}')

    def test_user_without_password(self) -> None:
        with pytest.raises(ValidationError, match="must have both user and password"):
            parse('{"default_system":"a","systems":{"a":{"host":"https://h","user":"u"}}}')

    def test_password_without_user(self) -> None:
        with pytest.raises(ValidationError, match="must have both user and password"):
            parse('{"default_system":"a","systems":{"a":{"host":"https://h","password":"p"}}}')

    def test_invalid_language(self) -> None:
        with pytest.raises(ValidationError):
            parse(
                '{"default_system":"a","systems":{"a":{"host":"https://h","user":"u","password":"p","language":"FR"}}}'
            )


class TestLoadYAMLFixture:
    """Parse the shared testdata/systems.yaml — same assertions as JSON tests."""

    def test_load_yaml_fixture(self) -> None:
        cfg = load(TESTDATA_YAML)

        assert cfg.default_system == "dev"
        assert len(cfg.systems) == 3

        dev = cfg.systems["dev"]
        assert dev.host == "https://dev-sap.example.com:44300"
        assert dev.client == "100"
        assert dev.user == "DEV_USER"
        assert dev.password.get_secret_value() == "dev_secret"
        assert dev.language == "DE"
        assert dev.tls_skip_verify is True

        oauth = cfg.systems["oauth"]
        assert oauth.is_oauth2 is True
        assert oauth.oauth2_client_id == "my-mcp-client"


class TestYAMLMatchesJSON:
    def test_yaml_and_json_produce_same_config(self) -> None:
        json_cfg = load(TESTDATA)
        yaml_cfg = load(TESTDATA_YAML)

        assert json_cfg.default_system == yaml_cfg.default_system
        assert len(json_cfg.systems) == len(yaml_cfg.systems)
        for name, json_sys in json_cfg.systems.items():
            yaml_sys = yaml_cfg.systems[name]
            assert json_sys.host == yaml_sys.host
            assert json_sys.client == yaml_sys.client
            assert json_sys.user == yaml_sys.user
            assert json_sys.password.get_secret_value() == yaml_sys.password.get_secret_value()
            assert json_sys.language == yaml_sys.language
            assert json_sys.tls_skip_verify == yaml_sys.tls_skip_verify
            assert json_sys.oauth2_client_id == yaml_sys.oauth2_client_id


class TestParseYAML:
    def test_parse_yaml_minimal(self) -> None:
        data = "default_system: s\nsystems:\n  s:\n    host: 'https://x:443'\n    client: '100'\n    user: u\n    password: p\n"
        cfg = parse_yaml(data)
        assert len(cfg.systems) == 1
        assert cfg.systems["s"].language == "EN"  # default applied

    def test_parse_yaml_invalid(self) -> None:
        with pytest.raises(ValueError, match="expected a YAML mapping"):
            parse_yaml("just a string")


class TestYAMLUnquotedClient:
    """YAML users may write client: 100 (no quotes). Must coerce to string."""

    def test_unquoted_client_coerced_to_string(self) -> None:
        data = "default_system: s\nsystems:\n  s:\n    host: 'https://x:443'\n    client: 100\n    user: u\n    password: p\n"
        cfg = parse_yaml(data)
        assert cfg.systems["s"].client == "100"
        assert isinstance(cfg.systems["s"].client, str)


class TestYAMLSpecialCharacters:
    """YAML has its own special characters (:, #, etc.) that need quoting."""

    def test_special_characters_yaml(self) -> None:
        testdata_special_yaml = Path(__file__).resolve().parent.parent / "testdata" / "special_characters.yaml"
        cfg = load(testdata_special_yaml)
        assert cfg.systems["tricky"].password.get_secret_value() == "p@ss:word#with!special&chars"
        assert cfg.systems["backslash"].user == "DOMAIN\\USER"
        assert cfg.systems["backslash"].password.get_secret_value() == "pass\\word\\with\\backslashes"


class TestLoadYMLExtension:
    def test_yml_extension_detected(self, tmp_path: Path) -> None:
        src = TESTDATA_YAML.read_bytes()
        yml_file = tmp_path / "config.yml"
        yml_file.write_bytes(src)
        cfg = load(yml_file)
        assert cfg.default_system == "dev"


class TestFrozenModels:
    def test_config_is_immutable(self) -> None:
        cfg = load(TESTDATA)
        with pytest.raises(ValidationError):
            cfg.default_system = "other"  # type: ignore[misc]

    def test_system_is_immutable(self) -> None:
        cfg = load(TESTDATA)
        with pytest.raises(ValidationError):
            cfg.systems["dev"].user = "hacked"  # type: ignore[misc]


class TestParseMinimal:
    def test_minimal(self) -> None:
        data = (
            '{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}'
        )
        cfg = parse(data)
        assert len(cfg.systems) == 1


class TestLoadFileNotFound:
    def test_file_not_found(self) -> None:
        with pytest.raises(FileNotFoundError):
            load("nonexistent.json")


class TestLoadDefault:
    def test_load_default_uses_env_var(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("SAP_CONFIG_FILE", str(TESTDATA))
        cfg = load_default()
        assert cfg.default_system == "dev"


class TestReadmeExample:
    """Verify that the README example behaves as documented."""

    def test_multiple_errors_at_once(self) -> None:
        """The README claims that multiple problems are reported in a single error."""
        bad_config = '{"default_system":"missing","systems":{"dev":{"host":"ftp://wrong","client":"1","user":"u"}}}'
        with pytest.raises(ValidationError) as exc_info:
            parse(bad_config)
        msg = str(exc_info.value)
        assert "host must start with http" in msg
        assert "client must be a 3-digit string" in msg
        assert "must have both user and password" in msg

    def test_load_default_file_not_found(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The README shows catching FileNotFoundError from load_default."""
        monkeypatch.setenv("SAP_CONFIG_FILE", "/nonexistent/path/systems.json")
        with pytest.raises(FileNotFoundError):
            load_default()


class TestExtensibility:
    """Verify that the shared config can be extended with custom fields (README example)."""

    def test_extend_config_with_custom_system(self) -> None:
        """Consumers can subclass SAPSystem to add project-specific fields."""

        class MySAPSystem(SAPSystem):
            """Extended system with a custom field."""

            model_config = {}  # unfreeze for subclass
            connection_name: str = ""

        sys = MySAPSystem(
            host="https://sap:44300",
            client="100",
            user="u",
            password="p",
            connection_name="HF S/4",
        )
        assert sys.connection_name == "HF S/4"
        assert sys.host == "https://sap:44300"
        assert sys.password.get_secret_value() == "p"
