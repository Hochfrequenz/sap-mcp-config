"""Tests for sap_mcp_config — must stay consistent with config_test.go."""

import json
from pathlib import Path

import pytest
from pydantic import ValidationError

from sap_mcp_config import SAPSystem, load, load_default, parse, parse_yaml

TESTDATA = Path(__file__).resolve().parent.parent / "testdata" / "systems.json"
TESTDATA_YAML = Path(__file__).resolve().parent.parent / "testdata" / "systems.yaml"
TESTDATA_SPECIAL = Path(__file__).resolve().parent.parent / "testdata" / "special_characters.json"
TESTDATA_ENV = Path(__file__).resolve().parent.parent / "testdata" / "env_placeholders.json"
TESTDATA_ENV_YAML = Path(__file__).resolve().parent.parent / "testdata" / "env_placeholders.yaml"

#: Environment for testdata/env_placeholders.* — kept identical in config_test.go.
ENV_FIXTURE_VARS = {
    "SAP_MCP_TEST_HOSTNAME": "sap.example.com",
    "SAP_MCP_TEST_PORT": "44300",
    "SAP_MCP_TEST_CLIENT": "100",
    "SAP_MCP_TEST_USER": "FIXTURE_USER",
    "SAP_MCP_TEST_PASSWORD": "fixture_secret",
}


@pytest.fixture(name="env_fixture")
def _env_fixture(monkeypatch: pytest.MonkeyPatch) -> None:
    """Set the variables that testdata/env_placeholders.* refers to."""
    for key, value in ENV_FIXTURE_VARS.items():
        monkeypatch.setenv(key, value)


class TestLoadTestFixture:
    """Parse the shared testdata/systems.json — same assertions as the Go tests."""

    def test_load_fixture(self) -> None:
        cfg = load(TESTDATA)

        assert cfg.default_system == "dev"
        assert len(cfg.systems) == 3

        dev = cfg.systems["dev"]
        assert dev.connection_name == "DEV - ERP Development"
        assert dev.host == "https://dev-sap.example.com:44300"
        assert dev.client == "100"
        assert dev.user == "DEV_USER"
        assert dev.password.get_secret_value() == "dev_secret"
        assert dev.language == "DE"
        assert dev.tls_skip_verify is True
        assert dev.is_oauth2 is False

        prod = cfg.systems["prod"]
        assert prod.connection_name == "PROD - ERP Production"
        assert prod.host == "https://prod-sap.example.com:44300"
        assert prod.client == "200"
        assert prod.user == "PROD_USER"
        assert prod.password.get_secret_value() == "prod_secret"
        assert prod.language == "EN"
        assert prod.tls_skip_verify is False
        assert prod.is_oauth2 is False

        oauth = cfg.systems["oauth"]
        assert oauth.connection_name == "OAuth System"
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


class TestStandaloneSAPSystem:
    """SAPSystem is exported and documented as subclassable, so it must validate itself."""

    def test_bad_language_rejected(self) -> None:
        with pytest.raises(ValidationError, match='language must be "DE" or "EN"'):
            SAPSystem(host="https://x", language="fr")

    def test_good_language_accepted(self) -> None:
        assert SAPSystem(host="https://x", language="de").language == "DE"

    def test_default_language_accepted(self) -> None:
        assert SAPSystem(host="https://x").language == "EN"


class TestLoadYAMLFixture:
    """Parse the shared testdata/systems.yaml — same assertions as JSON tests."""

    def test_load_yaml_fixture(self) -> None:
        cfg = load(TESTDATA_YAML)

        assert cfg.default_system == "dev"
        assert len(cfg.systems) == 3

        dev = cfg.systems["dev"]
        assert dev.connection_name == "DEV - ERP Development"
        assert dev.host == "https://dev-sap.example.com:44300"
        assert dev.client == "100"
        assert dev.user == "DEV_USER"
        assert dev.password.get_secret_value() == "dev_secret"
        assert dev.language == "DE"
        assert dev.tls_skip_verify is True

        oauth = cfg.systems["oauth"]
        assert oauth.connection_name == "OAuth System"
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
            assert json_sys.connection_name == yaml_sys.connection_name
            assert json_sys.host == yaml_sys.host
            assert json_sys.client == yaml_sys.client
            assert json_sys.user == yaml_sys.user
            assert json_sys.password.get_secret_value() == yaml_sys.password.get_secret_value()
            assert json_sys.language == yaml_sys.language
            assert json_sys.tls_skip_verify == yaml_sys.tls_skip_verify
            assert json_sys.oauth2_client_id == yaml_sys.oauth2_client_id


class TestParseYAML:
    def test_parse_yaml_minimal(self) -> None:
        data = (
            "default_system: s\nsystems:\n  s:\n"
            "    host: 'https://x:443'\n    client: '100'\n    user: u\n    password: p\n"
        )
        cfg = parse_yaml(data)
        assert len(cfg.systems) == 1
        assert cfg.systems["s"].language == "EN"  # default applied

    def test_parse_yaml_invalid(self) -> None:
        with pytest.raises(ValueError, match="expected a YAML mapping"):
            parse_yaml("just a string")


class TestYAMLUnquotedClient:
    """YAML users may write client: 100 (no quotes). Must coerce to string."""

    def test_unquoted_client_coerced_to_string(self) -> None:
        data = (
            "default_system: s\nsystems:\n  s:\n"
            "    host: 'https://x:443'\n    client: 100\n    user: u\n    password: p\n"
        )
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


class TestEnvPlaceholders:
    """``${env:VAR}`` interpolation — same assertions as the Go tests."""

    @pytest.mark.usefixtures("env_fixture")
    def test_placeholders_resolved_from_env(self) -> None:
        cfg = load(TESTDATA_ENV)
        system = cfg.systems["interpolated"]

        # A placeholder may be the whole value...
        assert system.user == "FIXTURE_USER"
        assert system.password.get_secret_value() == "fixture_secret"
        assert system.client == "100"
        # ...or embedded, several times, inside a larger string.
        assert system.host == "https://sap.example.com:44300"
        assert system.language == "DE"

    @pytest.mark.usefixtures("env_fixture")
    def test_non_identifier_is_left_literal(self) -> None:
        """``${env:not an identifier}`` does not match the grammar, so it stays as typed."""
        cfg = load(TESTDATA_ENV)
        assert cfg.systems["interpolated"].connection_name == "literal ${env:not an identifier} stays"

    @pytest.mark.usefixtures("env_fixture")
    def test_systems_without_placeholders_are_untouched(self) -> None:
        cfg = load(TESTDATA_ENV)
        plain = cfg.systems["plain"]
        assert plain.host == "https://plain-sap.example.com:44300"
        assert plain.user == "PLAIN_USER"
        assert plain.password.get_secret_value() == "plain_secret"

    @pytest.mark.usefixtures("env_fixture")
    def test_yaml_matches_json(self) -> None:
        json_cfg = load(TESTDATA_ENV)
        yaml_cfg = load(TESTDATA_ENV_YAML)
        for name, json_sys in json_cfg.systems.items():
            yaml_sys = yaml_cfg.systems[name]
            assert json_sys.host == yaml_sys.host
            assert json_sys.client == yaml_sys.client
            assert json_sys.user == yaml_sys.user
            assert json_sys.password.get_secret_value() == yaml_sys.password.get_secret_value()
            assert json_sys.connection_name == yaml_sys.connection_name

    @pytest.mark.parametrize(
        "literal",
        [
            "${env:not an identifier}",  # spaces are not allowed in a name
            "${env:2FA_TOKEN}",  # a name cannot start with a digit
            "${SAP_PASSWORD}",  # missing the env: prefix
            "$env:SAP_PASSWORD",  # missing the braces
        ],
    )
    def test_near_misses_are_kept_verbatim(self, literal: str, monkeypatch: pytest.MonkeyPatch) -> None:
        """Text that only looks like a placeholder is used as-is — see the README table."""
        monkeypatch.setenv("SAP_PASSWORD", "should-not-be-used")
        monkeypatch.setenv("SAP_MCP_TEST_2FA_TOKEN", "should-not-be-used")
        data = json.dumps(
            {
                "default_system": "a",
                "systems": {"a": {"host": "https://h", "client": "100", "user": "u", "password": literal}},
            }
        )
        cfg = parse(data)
        assert cfg.systems["a"].password.get_secret_value() == literal

    def test_unset_variable_is_an_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        for key in ENV_FIXTURE_VARS:
            monkeypatch.delenv(key, raising=False)
        with pytest.raises(ValidationError, match=r"references \$\{env:SAP_MCP_TEST_HOSTNAME\}, which is not set"):
            load(TESTDATA_ENV)

    def test_unset_user_and_password_do_not_become_oauth2(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A forgotten export must fail loudly, not silently switch the system to OAuth2."""
        for key in ENV_FIXTURE_VARS:
            monkeypatch.delenv(key, raising=False)
        with pytest.raises(ValidationError) as exc_info:
            load(TESTDATA_ENV)
        msg = str(exc_info.value)
        assert "user references ${env:SAP_MCP_TEST_USER}" in msg
        assert "password references ${env:SAP_MCP_TEST_PASSWORD}" in msg
        # The both-or-neither rule must not fire on unresolved values.
        assert "must have both user and password" not in msg

    def test_unset_errors_collected_with_other_errors(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("SAP_MCP_TEST_PW", raising=False)
        data = (
            '{"default_system":"a","systems":{"a":'
            '{"host":"ftp://wrong","client":"100","user":"u","password":"${env:SAP_MCP_TEST_PW}"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        msg = str(exc_info.value)
        assert "password references ${env:SAP_MCP_TEST_PW}, which is not set" in msg
        assert "host must start with http" in msg

    def test_error_does_not_echo_resolved_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A value taken from the environment is never printed back, even when it fails a check."""
        monkeypatch.setenv("SAP_MCP_TEST_BAD_HOST", "ftp://secret-internal-host")
        monkeypatch.setenv("SAP_MCP_TEST_BAD_CLIENT", "SECRET123")
        monkeypatch.setenv("SAP_MCP_TEST_BAD_LANG", "TOPSECRET")
        data = json.dumps(
            {
                "default_system": "a",
                "systems": {
                    "a": {
                        "host": "${env:SAP_MCP_TEST_BAD_HOST}",
                        "client": "${env:SAP_MCP_TEST_BAD_CLIENT}",
                        "user": "u",
                        "password": "p",
                        "language": "${env:SAP_MCP_TEST_BAD_LANG}",
                    }
                },
            }
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        # Assert on our own message, not str(exc): pydantic separately echoes the
        # validator's raw input, which is a pre-existing leak tracked elsewhere.
        message = exc_info.value.errors()[0]["msg"]
        assert "secret-internal-host" not in message
        assert "SECRET123" not in message
        assert "TOPSECRET" not in message
        assert "the value taken from the environment" in message

    def test_resolved_secret_never_reaches_the_exception(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """The whole point of the feature is that the secret is not in the file — nor in the error."""
        monkeypatch.setenv("SAP_MCP_TEST_PW_LEAK", "hunter2-env-secret")
        data = (
            '{"default_system":"dev","systems":{"dev":{"host":"https://x","client":"12345",'
            '"user":"U","password":"${env:SAP_MCP_TEST_PW_LEAK}"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        assert "hunter2-env-secret" not in str(exc_info.value)
        # pydantic echoes the validator's input by default; it must be withheld.
        assert exc_info.value.errors()[0]["input"] is None

    def test_env_derived_default_system_is_not_echoed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("SAP_MCP_TEST_DEF_NAME", "secret-system-name")
        data = (
            '{"default_system":"${env:SAP_MCP_TEST_DEF_NAME}","systems":{"dev":'
            '{"host":"https://x","client":"100","user":"u","password":"p"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert "secret-system-name" not in message
        assert "the value taken from the environment" in message

    def test_system_named_empty_string_is_still_prefixed(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A system literally named "" must not look like a top-level field error."""
        monkeypatch.delenv("SAP_MCP_TEST_EMPTY_NAME", raising=False)
        data = (
            '{"default_system":"dev","systems":{"":{"host":"${env:SAP_MCP_TEST_EMPTY_NAME}"},'
            '"dev":{"host":"https://x","client":"100","user":"u","password":"p"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert 'system "": host references ${env:SAP_MCP_TEST_EMPTY_NAME}' in message

    def test_dotenv_supplies_placeholders(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        """A .env in the working directory feeds placeholders, as Go's godotenv does."""
        (tmp_path / ".env").write_text("SAP_MCP_TEST_DOTENV_PW=from-dotenv\n", encoding="utf-8")
        (tmp_path / "systems.json").write_text(
            '{"default_system":"a","systems":{"a":{"host":"https://h","client":"100",'
            '"user":"u","password":"${env:SAP_MCP_TEST_DOTENV_PW}"}}}',
            encoding="utf-8",
        )
        monkeypatch.delenv("SAP_MCP_TEST_DOTENV_PW", raising=False)
        monkeypatch.setenv("SAP_CONFIG_FILE", str(tmp_path / "systems.json"))
        monkeypatch.chdir(tmp_path)
        cfg = load_default()
        assert cfg.systems["a"].password.get_secret_value() == "from-dotenv"

    def test_message_quoting_escapes_like_go(self) -> None:
        """A quote or newline in a value must not break the message apart — and must match Go's %q."""
        data = json.dumps(
            {
                "default_system": "a",
                "systems": {"a": {"host": 'ftp://ho"st\nline', "client": "100", "user": "u", "password": "p"}},
            }
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        # Byte-identical to the Go assertion in TestErrorQuotingEscapesSpecialCharacters.
        assert r'got "ftp://ho\"st\nline"' in exc_info.value.errors()[0]["msg"]

    def test_literal_values_are_still_echoed(self) -> None:
        """A value written in the file is not a secret we hid from the user — keep echoing it."""
        data = '{"default_system":"a","systems":{"a":{"host":"ftp://written-in-file","user":"u","password":"p"}}}'
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        assert "ftp://written-in-file" in exc_info.value.errors()[0]["msg"]

    def test_env_value_containing_placeholder_is_literal(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Single-pass holds at the validation layer: an inner ${env:...} is literal text."""
        monkeypatch.setenv("SAP_MCP_TEST_SECRET_WITH_PH", "hunter2${env:SAP_MCP_TEST_NEVER_SET}tail")
        monkeypatch.delenv("SAP_MCP_TEST_NEVER_SET", raising=False)
        data = (
            '{"default_system":"a","systems":{"a":'
            '{"host":"https://h","client":"100","user":"u","password":"${env:SAP_MCP_TEST_SECRET_WITH_PH}"}}}'
        )
        cfg = parse(data)
        assert cfg.systems["a"].password.get_secret_value() == "hunter2${env:SAP_MCP_TEST_NEVER_SET}tail"

    def test_empty_variable_is_an_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A defined-but-empty variable — an unpopulated CI secret — must not become OAuth2."""
        monkeypatch.setenv("SAP_MCP_TEST_EMPTY_USER", "")
        monkeypatch.setenv("SAP_MCP_TEST_EMPTY_PW", "")
        data = (
            '{"default_system":"a","systems":{"a":{"host":"https://h","client":"100",'
            '"user":"${env:SAP_MCP_TEST_EMPTY_USER}","password":"${env:SAP_MCP_TEST_EMPTY_PW}"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert "references ${env:SAP_MCP_TEST_EMPTY_USER}, which is set but empty" in message
        assert "must have both user and password" not in message

    def test_unresolved_language_collected_with_other_errors(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """An unresolved language must not abort the rest of the report."""
        for key in ("SAP_MCP_TEST_UU", "SAP_MCP_TEST_PP", "SAP_MCP_TEST_LL"):
            monkeypatch.delenv(key, raising=False)
        data = (
            '{"default_system":"a","systems":{"a":{"host":"https://h","client":"100",'
            '"user":"${env:SAP_MCP_TEST_UU}","password":"${env:SAP_MCP_TEST_PP}",'
            '"language":"${env:SAP_MCP_TEST_LL}"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert "user references ${env:SAP_MCP_TEST_UU}" in message
        assert "password references ${env:SAP_MCP_TEST_PP}" in message
        assert "language references ${env:SAP_MCP_TEST_LL}" in message

    def test_unresolved_field_skips_its_other_checks(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("SAP_MCP_TEST_CL", raising=False)
        data = (
            '{"default_system":"a","systems":{"a":'
            '{"host":"https://h","client":"${env:SAP_MCP_TEST_CL}","user":"u","password":"p"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert "client references ${env:SAP_MCP_TEST_CL}" in message
        assert "3-digit" not in message

    def test_unresolved_in_all_placeholder_fields(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """connection_name and oauth2_client_id have no other validation to exercise them."""
        for key in ("SAP_MCP_TEST_CN", "SAP_MCP_TEST_OA"):
            monkeypatch.delenv(key, raising=False)
        data = (
            '{"default_system":"a","systems":{"a":{"connection_name":"${env:SAP_MCP_TEST_CN}",'
            '"host":"https://h","client":"100","oauth2_client_id":"${env:SAP_MCP_TEST_OA}"}}}'
        )
        with pytest.raises(ValidationError) as exc_info:
            parse(data)
        message = exc_info.value.errors()[0]["msg"]
        assert "connection_name references ${env:SAP_MCP_TEST_CN}" in message
        assert "oauth2_client_id references ${env:SAP_MCP_TEST_OA}" in message

    def test_unset_variable_is_an_error_yaml(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Mirror of the JSON unset path — the fixtures only exercised JSON."""
        for key in ENV_FIXTURE_VARS:
            monkeypatch.delenv(key, raising=False)
        with pytest.raises(ValidationError, match=r"references \$\{env:SAP_MCP_TEST_HOSTNAME\}, which is not set"):
            load(TESTDATA_ENV_YAML)

    def test_self_referential_yaml_does_not_recurse(self) -> None:
        """A cyclic anchor must not send interpolation into unbounded recursion."""
        data = (
            "x: &a [*a]\ndefault_system: a\nsystems:\n  a: {host: 'https://h', client: '100', user: u, password: p}\n"
        )
        cfg = parse_yaml(data)
        assert cfg.systems["a"].host == "https://h"

    def test_substitution_is_single_pass(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """A resolved value containing ${env:...} is not rescanned — no indirect reads."""
        monkeypatch.setenv("SAP_MCP_TEST_SECRET", "the-real-secret")
        monkeypatch.setenv("SAP_MCP_TEST_INDIRECT", "${env:SAP_MCP_TEST_SECRET}")
        data = (
            '{"default_system":"a","systems":{"a":'
            '{"host":"https://h","client":"100","user":"u","password":"${env:SAP_MCP_TEST_INDIRECT}"}}}'
        )
        cfg = parse(data)
        assert cfg.systems["a"].password.get_secret_value() == "${env:SAP_MCP_TEST_SECRET}"

    def test_unresolved_default_system_reported(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("SAP_MCP_TEST_DEFAULT", raising=False)
        data = (
            '{"default_system":"${env:SAP_MCP_TEST_DEFAULT}","systems":{"a":'
            '{"host":"https://h","client":"100","user":"u","password":"p"}}}'
        )
        with pytest.raises(ValidationError, match=r"default_system references \$\{env:SAP_MCP_TEST_DEFAULT\}"):
            parse(data)

    def test_placeholder_in_config_without_env_support_still_works(self) -> None:
        """Configs with no placeholders behave exactly as before."""
        cfg = load(TESTDATA)
        assert cfg.systems["dev"].password.get_secret_value() == "dev_secret"


class TestExtensibility:
    """Verify that the shared config can be extended with custom fields (README example)."""

    def test_extend_config_with_custom_system(self) -> None:
        """Consumers can subclass SAPSystem to add project-specific fields."""

        class MySAPSystem(SAPSystem):
            """Extended system with a custom field."""

            model_config = {}  # noqa: RUF012 -- unfreeze for subclass
            custom_timeout: int = 30

        sys = MySAPSystem(
            host="https://sap:44300",
            client="100",
            user="u",
            password="p",
            connection_name="HF S/4",
            custom_timeout=60,
        )
        assert sys.connection_name == "HF S/4"
        assert sys.custom_timeout == 60
        assert sys.host == "https://sap:44300"
        assert sys.password.get_secret_value() == "p"
