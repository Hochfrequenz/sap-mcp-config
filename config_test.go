package sapmcpconfig_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTestFixture(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/systems.json")
	require.NoError(t, err)

	assert.Equal(t, "dev", cfg.DefaultSystem)
	assert.Len(t, cfg.Systems, 3)

	dev := cfg.Systems["dev"]
	assert.Equal(t, "DEV - ERP Development", dev.ConnectionName)
	assert.Equal(t, "https://dev-sap.example.com:44300", dev.Host)
	assert.Equal(t, "100", dev.Client)
	assert.Equal(t, "DEV_USER", dev.User)
	assert.Equal(t, "dev_secret", dev.Password)
	assert.Equal(t, "DE", dev.Language)
	assert.True(t, dev.TLSSkipVerify)
	assert.False(t, dev.IsOAuth2())

	prod := cfg.Systems["prod"]
	assert.Equal(t, "PROD - ERP Production", prod.ConnectionName)
	assert.Equal(t, "https://prod-sap.example.com:44300", prod.Host)
	assert.Equal(t, "200", prod.Client)
	assert.Equal(t, "PROD_USER", prod.User)
	assert.Equal(t, "prod_secret", prod.Password)
	assert.Equal(t, "EN", prod.Language)
	assert.False(t, prod.TLSSkipVerify)
	assert.False(t, prod.IsOAuth2())

	oauth := cfg.Systems["oauth"]
	assert.Equal(t, "OAuth System", oauth.ConnectionName)
	assert.Equal(t, "https://oauth-sap.example.com:44300", oauth.Host)
	assert.Equal(t, "300", oauth.Client)
	assert.Equal(t, "", oauth.User)
	assert.Equal(t, "", oauth.Password)
	assert.Equal(t, "EN", oauth.Language)
	assert.Equal(t, "my-mcp-client", oauth.OAuth2ClientID)
	assert.True(t, oauth.IsOAuth2())
}

func TestGetDefault(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/systems.json")
	require.NoError(t, err)
	def := cfg.GetDefault()
	assert.Equal(t, "https://dev-sap.example.com:44300", def.Host)
}

func TestLanguageDefaultsToEN(t *testing.T) {
	data := `{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}`
	cfg, err := sapmcpconfig.Parse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "EN", cfg.Systems["s"].Language)
}

func TestLanguageCaseInsensitive(t *testing.T) {
	data := `{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p","language":"de"}}}`
	cfg, err := sapmcpconfig.Parse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "DE", cfg.Systems["s"].Language)
}

func TestParseValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "no systems",
			json:    `{"default_system":"x","systems":{}}`,
			wantErr: "no systems defined",
		},
		{
			name:    "null systems",
			json:    `{"default_system":"x","systems":null}`,
			wantErr: "no systems defined",
		},
		{
			name:    "default not found",
			json:    `{"default_system":"missing","systems":{"a":{"host":"https://h","user":"u","password":"p"}}}`,
			wantErr: `default_system "missing" not found`,
		},
		{
			name:    "missing host",
			json:    `{"default_system":"a","systems":{"a":{"client":"100","user":"u","password":"p"}}}`,
			wantErr: `system "a": host is required`,
		},
		{
			name:    "invalid host scheme",
			json:    `{"default_system":"a","systems":{"a":{"host":"ftp://h","client":"100","user":"u","password":"p"}}}`,
			wantErr: "host must start with http:// or https://",
		},
		{
			name:    "invalid client",
			json:    `{"default_system":"a","systems":{"a":{"host":"https://h","client":"1","user":"u","password":"p"}}}`,
			wantErr: `client must be a 3-digit string`,
		},
		{
			name:    "user without password",
			json:    `{"default_system":"a","systems":{"a":{"host":"https://h","user":"u"}}}`,
			wantErr: "must have both user and password",
		},
		{
			name:    "password without user",
			json:    `{"default_system":"a","systems":{"a":{"host":"https://h","password":"p"}}}`,
			wantErr: "must have both user and password",
		},
		{
			name:    "invalid language",
			json:    `{"default_system":"a","systems":{"a":{"host":"https://h","user":"u","password":"p","language":"FR"}}}`,
			wantErr: `language must be "DE" or "EN"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sapmcpconfig.Parse([]byte(tt.json))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseMinimal(t *testing.T) {
	data := `{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}`
	cfg, err := sapmcpconfig.Parse([]byte(data))
	require.NoError(t, err)
	assert.Len(t, cfg.Systems, 1)
}

func TestSpecialCharacterPasswords(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/special_characters.json")
	require.NoError(t, err)

	assert.Len(t, cfg.Systems, 3)

	tricky := cfg.Systems["tricky"]
	assert.Equal(t, `p@ss"w0rd'with<special>&chars!{}[]`, tricky.Password)

	unicode := cfg.Systems["unicode"]
	assert.Equal(t, "UMLAUT_ÜÖÄ", unicode.User)
	assert.Equal(t, "äöüß€£", unicode.Password)

	backslash := cfg.Systems["backslash"]
	assert.Equal(t, `DOMAIN\USER`, backslash.User)
	assert.Equal(t, `pass\word\with\backslashes`, backslash.Password)
}

func TestLoadYAMLFixture(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/systems.yaml")
	require.NoError(t, err)

	assert.Equal(t, "dev", cfg.DefaultSystem)
	assert.Len(t, cfg.Systems, 3)

	dev := cfg.Systems["dev"]
	assert.Equal(t, "DEV - ERP Development", dev.ConnectionName)
	assert.Equal(t, "https://dev-sap.example.com:44300", dev.Host)
	assert.Equal(t, "100", dev.Client)
	assert.Equal(t, "DEV_USER", dev.User)
	assert.Equal(t, "dev_secret", dev.Password)
	assert.Equal(t, "DE", dev.Language)
	assert.True(t, dev.TLSSkipVerify)

	oauth := cfg.Systems["oauth"]
	assert.Equal(t, "OAuth System", oauth.ConnectionName)
	assert.True(t, oauth.IsOAuth2())
	assert.Equal(t, "my-mcp-client", oauth.OAuth2ClientID)
}

func TestLoadYAMLMatchesJSON(t *testing.T) {
	jsonCfg, err := sapmcpconfig.Load("testdata/systems.json")
	require.NoError(t, err)

	yamlCfg, err := sapmcpconfig.Load("testdata/systems.yaml")
	require.NoError(t, err)

	// Same data, different format — must produce identical configs.
	assert.Equal(t, jsonCfg.DefaultSystem, yamlCfg.DefaultSystem)
	assert.Equal(t, len(jsonCfg.Systems), len(yamlCfg.Systems))
	for name, jsonSys := range jsonCfg.Systems {
		yamlSys, ok := yamlCfg.Systems[name]
		require.True(t, ok, "system %q missing from YAML config", name)
		assert.Equal(t, jsonSys, yamlSys, "system %q differs between JSON and YAML", name)
	}
}

func TestParseYAML(t *testing.T) {
	data := []byte("default_system: s\nsystems:\n  s:\n    host: \"https://x:443\"\n    client: \"100\"\n    user: u\n    password: p\n")
	cfg, err := sapmcpconfig.ParseYAML(data)
	require.NoError(t, err)
	assert.Len(t, cfg.Systems, 1)
	assert.Equal(t, "EN", cfg.Systems["s"].Language) // default applied
}

func TestYAMLUnquotedClient(t *testing.T) {
	data := []byte("default_system: s\nsystems:\n  s:\n    host: \"https://x:443\"\n    client: 100\n    user: u\n    password: p\n")
	cfg, err := sapmcpconfig.ParseYAML(data)
	require.NoError(t, err)
	assert.Equal(t, "100", cfg.Systems["s"].Client)
}

func TestYAMLSpecialCharacters(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/special_characters.yaml")
	require.NoError(t, err)

	tricky := cfg.Systems["tricky"]
	assert.Equal(t, "p@ss:word#with!special&chars", tricky.Password)

	backslash := cfg.Systems["backslash"]
	assert.Equal(t, `DOMAIN\USER`, backslash.User)
	assert.Equal(t, `pass\word\with\backslashes`, backslash.Password)
}

func TestLoadYMLExtension(t *testing.T) {
	// Copy the YAML fixture with .yml extension to verify extension detection.
	data, err := os.ReadFile("testdata/systems.yaml")
	require.NoError(t, err)
	tmp := t.TempDir() + "/config.yml"
	require.NoError(t, os.WriteFile(tmp, data, 0o644))

	cfg, err := sapmcpconfig.Load(tmp)
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.DefaultSystem)
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := sapmcpconfig.Load("nonexistent.json")
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestLoadDefaultUsesEnvVar(t *testing.T) {
	t.Setenv("SAP_CONFIG_FILE", "testdata/systems.json")
	cfg, err := sapmcpconfig.LoadDefault()
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.DefaultSystem)
}

func TestMultipleValidationErrors(t *testing.T) {
	// Two systems, both broken — both errors should appear.
	data := `{"default_system":"a","systems":{"a":{"host":""},"b":{"host":"","user":"u"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `system "a"`)
	assert.Contains(t, err.Error(), `system "b"`)
}

func TestReadmeExample(t *testing.T) {
	// The README claims that multiple problems are reported in a single error.
	data := `{"default_system":"missing","systems":{"dev":{"host":"ftp://wrong","client":"1","user":"u"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, `default_system "missing" not found`)
	assert.Contains(t, msg, "host must start with http")
	assert.Contains(t, msg, "client must be a 3-digit string")
	assert.Contains(t, msg, "must have both user and password")
}

func TestPasswordMaskedInString(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/systems.json")
	require.NoError(t, err)

	dev := cfg.Systems["dev"]
	str := dev.String()
	assert.NotContains(t, str, "dev_secret")
	assert.Contains(t, str, "***")
	assert.Contains(t, str, "ConnectionName:DEV - ERP Development")

	// Also check fmt.Sprintf which uses String() via Format()
	formatted := fmt.Sprintf("%v", dev)
	assert.NotContains(t, formatted, "dev_secret")

	// %+v also uses Format(), not the default struct printer
	verbose := fmt.Sprintf("%+v", dev)
	assert.NotContains(t, verbose, "dev_secret")
	assert.Contains(t, verbose, "***")
}

func TestPasswordAccessible(t *testing.T) {
	cfg, err := sapmcpconfig.Load("testdata/systems.json")
	require.NoError(t, err)
	assert.Equal(t, "dev_secret", cfg.Systems["dev"].Password)
}

// TestParseRejectsTrailingData pins that trailing data is rejected. Parse relies
// on json.Unmarshal for this; the test exists so a future hand-rolled decoder
// cannot quietly lose the behaviour.
func TestParseRejectsTrailingData(t *testing.T) {
	const valid = `{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}`

	for _, trailing := range []string{" true", "}", "]", ", {}", "garbage", `{"b":2}`} {
		t.Run(trailing, func(t *testing.T) {
			_, err := sapmcpconfig.Parse([]byte(valid + trailing))
			require.Error(t, err)
		})
	}

	t.Run("trailing whitespace is fine", func(t *testing.T) {
		cfg, err := sapmcpconfig.Parse([]byte(valid + "\n  \n"))
		require.NoError(t, err)
		assert.Len(t, cfg.Systems, 1)
	})
}

// TestYAMLScalarsSurviveInterpolation guards against interpolation changing a
// value's text. Decoding YAML into a generic document and re-encoding it
// re-tags unquoted scalars, so a password of 007 would silently load as 7.
func TestYAMLScalarsSurviveInterpolation(t *testing.T) {
	for _, raw := range []string{"007", "0100", "010", "0x1F", "1_000", "+1", "1e3", "1.10", "2001-12-14", "on"} {
		t.Run(raw, func(t *testing.T) {
			y := "default_system: a\nsystems:\n  a:\n    host: 'https://h'\n    client: '100'\n    user: u\n    password: " + raw + "\n"
			cfg, err := sapmcpconfig.ParseYAML([]byte(y))
			require.NoError(t, err)
			assert.Equal(t, raw, cfg.Systems["a"].Password, "password text must survive verbatim")
		})
	}
}

// TestEnvValueContainingPlaceholderIsLiteral pins the single-pass guarantee at
// the validation layer too: a secret whose value contains ${env:...} is literal
// text, whether or not that inner variable happens to exist.
func TestEnvValueContainingPlaceholderIsLiteral(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_SECRET_WITH_PH", "hunter2${env:SAP_MCP_TEST_NEVER_SET}tail")
	t.Setenv("SAP_MCP_TEST_NEVER_SET", "")
	require.NoError(t, os.Unsetenv("SAP_MCP_TEST_NEVER_SET"))

	data := `{"default_system":"a","systems":{"a":{"host":"https://h","client":"100","user":"u","password":"${env:SAP_MCP_TEST_SECRET_WITH_PH}"}}}`
	cfg, err := sapmcpconfig.Parse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "hunter2${env:SAP_MCP_TEST_NEVER_SET}tail", cfg.Systems["a"].Password)
}

// TestEnvPlaceholderEmptyValueIsError pins that a defined-but-empty variable is
// rejected. It is the shape an unpopulated CI secret takes, and letting it
// through would flip a credentialed system to OAuth2.
func TestEnvPlaceholderEmptyValueIsError(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_EMPTY_USER", "")
	t.Setenv("SAP_MCP_TEST_EMPTY_PW", "")

	data := `{"default_system":"a","systems":{"a":{"host":"https://h","client":"100","user":"${env:SAP_MCP_TEST_EMPTY_USER}","password":"${env:SAP_MCP_TEST_EMPTY_PW}"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references ${env:SAP_MCP_TEST_EMPTY_USER}, which is set but empty")
	assert.NotContains(t, err.Error(), "must have both user and password")
}

// TestEnvErrorsDoNotEchoResolvedValues pins that a value taken from the
// environment is never printed back, even when it fails a shape check.
func TestEnvErrorsDoNotEchoResolvedValues(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_BAD_HOST", "ftp://secret-internal-host")
	t.Setenv("SAP_MCP_TEST_BAD_CLIENT", "SECRET123")
	t.Setenv("SAP_MCP_TEST_BAD_LANG", "TOPSECRET")

	data := `{"default_system":"a","systems":{"a":{"host":"${env:SAP_MCP_TEST_BAD_HOST}","client":"${env:SAP_MCP_TEST_BAD_CLIENT}","user":"u","password":"p","language":"${env:SAP_MCP_TEST_BAD_LANG}"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, "secret-internal-host")
	assert.NotContains(t, msg, "SECRET123")
	assert.NotContains(t, msg, "TOPSECRET")
	assert.Contains(t, msg, "the value taken from the environment")
	// A value written literally in the file is still echoed, as before.
	literal := `{"default_system":"a","systems":{"a":{"host":"ftp://written-in-file","user":"u","password":"p"}}}`
	_, err = sapmcpconfig.Parse([]byte(literal))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ftp://written-in-file")
}

// TestUnresolvedLanguageCollectedWithOtherErrors pins that an unresolved
// language does not abort the rest of the report.
func TestUnresolvedLanguageCollectedWithOtherErrors(t *testing.T) {
	for _, k := range []string{"SAP_MCP_TEST_UU", "SAP_MCP_TEST_PP", "SAP_MCP_TEST_LL"} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	data := `{"default_system":"a","systems":{"a":{"host":"https://h","client":"100","user":"${env:SAP_MCP_TEST_UU}","password":"${env:SAP_MCP_TEST_PP}","language":"${env:SAP_MCP_TEST_LL}"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "user references ${env:SAP_MCP_TEST_UU}")
	assert.Contains(t, msg, "password references ${env:SAP_MCP_TEST_PP}")
	assert.Contains(t, msg, "language references ${env:SAP_MCP_TEST_LL}")
}

// TestUnresolvedFieldSkipsItsOtherChecks pins the suppression: an unresolved
// client must not also produce the 3-digit complaint.
func TestUnresolvedFieldSkipsItsOtherChecks(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_CL", "")
	require.NoError(t, os.Unsetenv("SAP_MCP_TEST_CL"))
	data := `{"default_system":"a","systems":{"a":{"host":"https://h","client":"${env:SAP_MCP_TEST_CL}","user":"u","password":"p"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client references ${env:SAP_MCP_TEST_CL}")
	assert.NotContains(t, err.Error(), "3-digit")
}

// TestUnresolvedInAllPlaceholderFields covers the fields with no other
// validation, which would otherwise never be exercised.
func TestUnresolvedInAllPlaceholderFields(t *testing.T) {
	for _, k := range []string{"SAP_MCP_TEST_CN", "SAP_MCP_TEST_OA"} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	data := `{"default_system":"a","systems":{"a":{"connection_name":"${env:SAP_MCP_TEST_CN}","host":"https://h","client":"100","oauth2_client_id":"${env:SAP_MCP_TEST_OA}"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection_name references ${env:SAP_MCP_TEST_CN}")
	assert.Contains(t, err.Error(), "oauth2_client_id references ${env:SAP_MCP_TEST_OA}")
}

// TestEnvDerivedDefaultSystemIsNotEchoed pins that default_system gets the same
// withholding treatment as every other field.
func TestEnvDerivedDefaultSystemIsNotEchoed(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_DEF_NAME", "secret-system-name")
	data := `{"default_system":"${env:SAP_MCP_TEST_DEF_NAME}","systems":{"dev":{"host":"https://x","client":"100","user":"u","password":"p"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-system-name")
	assert.Contains(t, err.Error(), "the value taken from the environment")
}

// TestSystemNamedEmptyStringIsStillPrefixed guards the sentinel: a system
// literally named "" must not render like a top-level field error.
func TestSystemNamedEmptyStringIsStillPrefixed(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_EMPTY_NAME", "")
	require.NoError(t, os.Unsetenv("SAP_MCP_TEST_EMPTY_NAME"))
	data := `{"default_system":"dev","systems":{"":{"host":"${env:SAP_MCP_TEST_EMPTY_NAME}"},"dev":{"host":"https://x","client":"100","user":"u","password":"p"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `system "": host references ${env:SAP_MCP_TEST_EMPTY_NAME}`)
}

// TestInvalidLanguageReportsNormalizedValue keeps the message identical to
// Python's, whose BeforeValidator has already uppercased the field.
func TestInvalidLanguageReportsNormalizedValue(t *testing.T) {
	data := `{"default_system":"a","systems":{"a":{"host":"https://h","user":"u","password":"p","language":"fr"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `language must be "DE" or "EN", got "FR"`)
}

// TestErrorOrderIsDeterministic guards against Go's randomized map iteration
// leaking into the reported message order, which Python never does.
func TestErrorOrderIsDeterministic(t *testing.T) {
	data := `{"default_system":"a","systems":{"a":{"host":"ftp://one"},"b":{"host":"ftp://two"},"c":{"host":"ftp://three"},"d":{"host":"ftp://four"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	first := err.Error()
	for i := 0; i < 50; i++ {
		_, err := sapmcpconfig.Parse([]byte(data))
		require.Error(t, err)
		require.Equal(t, first, err.Error(), "message order must not vary between runs")
	}
}

// TestEnvPlaceholderUnsetIsErrorYAML mirrors the JSON unset path, which was the
// only one the fixtures exercised.
func TestEnvPlaceholderUnsetIsErrorYAML(t *testing.T) {
	unsetEnvFixture(t)
	_, err := sapmcpconfig.Load("testdata/env_placeholders.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references ${env:SAP_MCP_TEST_HOSTNAME}, which is not set in the environment")
}

// envFixtureVars is the environment for testdata/env_placeholders.* — kept
// identical in unittests/test_models.py.
var envFixtureVars = map[string]string{
	"SAP_MCP_TEST_HOSTNAME": "sap.example.com",
	"SAP_MCP_TEST_PORT":     "44300",
	"SAP_MCP_TEST_CLIENT":   "100",
	"SAP_MCP_TEST_USER":     "FIXTURE_USER",
	"SAP_MCP_TEST_PASSWORD": "fixture_secret",
}

// setEnvFixture sets the variables that testdata/env_placeholders.* refers to.
func setEnvFixture(t *testing.T) {
	t.Helper()
	for key, value := range envFixtureVars {
		t.Setenv(key, value)
	}
}

// unsetEnvFixture removes them again. t.Setenv is called first so the original
// value is restored when the test finishes.
func unsetEnvFixture(t *testing.T) {
	t.Helper()
	for key := range envFixtureVars {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestEnvPlaceholdersResolved(t *testing.T) {
	setEnvFixture(t)
	cfg, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.NoError(t, err)

	sys := cfg.Systems["interpolated"]
	// A placeholder may be the whole value...
	assert.Equal(t, "FIXTURE_USER", sys.User)
	assert.Equal(t, "fixture_secret", sys.Password)
	assert.Equal(t, "100", sys.Client)
	// ...or embedded, several times, inside a larger string.
	assert.Equal(t, "https://sap.example.com:44300", sys.Host)
	assert.Equal(t, "DE", sys.Language)
}

func TestEnvPlaceholderNonIdentifierIsLiteral(t *testing.T) {
	setEnvFixture(t)
	cfg, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.NoError(t, err)
	// ${env:not an identifier} does not match the grammar, so it stays as typed.
	assert.Equal(t, "literal ${env:not an identifier} stays", cfg.Systems["interpolated"].ConnectionName)
}

func TestEnvPlaceholdersLeaveOtherSystemsUntouched(t *testing.T) {
	setEnvFixture(t)
	cfg, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.NoError(t, err)

	plain := cfg.Systems["plain"]
	assert.Equal(t, "https://plain-sap.example.com:44300", plain.Host)
	assert.Equal(t, "PLAIN_USER", plain.User)
	assert.Equal(t, "plain_secret", plain.Password)
}

func TestEnvPlaceholdersYAMLMatchesJSON(t *testing.T) {
	setEnvFixture(t)
	jsonCfg, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.NoError(t, err)
	yamlCfg, err := sapmcpconfig.Load("testdata/env_placeholders.yaml")
	require.NoError(t, err)

	for name, jsonSys := range jsonCfg.Systems {
		yamlSys, ok := yamlCfg.Systems[name]
		require.True(t, ok, "system %q missing from YAML config", name)
		assert.Equal(t, jsonSys, yamlSys, "system %q differs between JSON and YAML", name)
	}
}

// TestEnvPlaceholderNearMissesKeptVerbatim pins the README table: text that only
// looks like a placeholder is used as-is rather than replaced or rejected.
func TestEnvPlaceholderNearMissesKeptVerbatim(t *testing.T) {
	t.Setenv("SAP_PASSWORD", "should-not-be-used")

	for _, literal := range []string{
		"${env:not an identifier}", // spaces are not allowed in a name
		"${env:2FA_TOKEN}",         // a name cannot start with a digit
		"${SAP_PASSWORD}",          // missing the env: prefix
		"$env:SAP_PASSWORD",        // missing the braces
	} {
		t.Run(literal, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{
				"default_system": "a",
				"systems": map[string]any{
					"a": map[string]any{"host": "https://h", "client": "100", "user": "u", "password": literal},
				},
			})
			require.NoError(t, err)
			cfg, err := sapmcpconfig.Parse(data)
			require.NoError(t, err)
			assert.Equal(t, literal, cfg.Systems["a"].Password)
		})
	}
}

func TestEnvPlaceholderUnsetIsError(t *testing.T) {
	unsetEnvFixture(t)
	_, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "references ${env:SAP_MCP_TEST_HOSTNAME}, which is not set in the environment")
}

func TestEnvPlaceholderUnsetDoesNotBecomeOAuth2(t *testing.T) {
	// A forgotten export must fail loudly, not silently switch the system to OAuth2.
	unsetEnvFixture(t)
	_, err := sapmcpconfig.Load("testdata/env_placeholders.json")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "user references ${env:SAP_MCP_TEST_USER}")
	assert.Contains(t, msg, "password references ${env:SAP_MCP_TEST_PASSWORD}")
	// The both-or-neither rule must not fire on unresolved values.
	assert.NotContains(t, msg, "must have both user and password")
}

func TestEnvPlaceholderUnsetCollectedWithOtherErrors(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_PW", "")
	require.NoError(t, os.Unsetenv("SAP_MCP_TEST_PW"))
	data := `{"default_system":"a","systems":{"a":{"host":"ftp://wrong","client":"100","user":"u","password":"${env:SAP_MCP_TEST_PW}"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "password references ${env:SAP_MCP_TEST_PW}, which is not set")
	assert.Contains(t, msg, "host must start with http")
}

func TestEnvSubstitutionIsSinglePass(t *testing.T) {
	// A resolved value containing ${env:...} is not rescanned — no indirect reads.
	t.Setenv("SAP_MCP_TEST_SECRET", "the-real-secret")
	t.Setenv("SAP_MCP_TEST_INDIRECT", "${env:SAP_MCP_TEST_SECRET}")
	data := `{"default_system":"a","systems":{"a":{"host":"https://h","client":"100","user":"u","password":"${env:SAP_MCP_TEST_INDIRECT}"}}}`
	cfg, err := sapmcpconfig.Parse([]byte(data))
	require.NoError(t, err)
	assert.Equal(t, "${env:SAP_MCP_TEST_SECRET}", cfg.Systems["a"].Password)
}

func TestEnvPlaceholderInDefaultSystem(t *testing.T) {
	t.Setenv("SAP_MCP_TEST_DEFAULT", "")
	require.NoError(t, os.Unsetenv("SAP_MCP_TEST_DEFAULT"))
	data := `{"default_system":"${env:SAP_MCP_TEST_DEFAULT}","systems":{"a":{"host":"https://h","client":"100","user":"u","password":"p"}}}`
	_, err := sapmcpconfig.Parse([]byte(data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_system references ${env:SAP_MCP_TEST_DEFAULT}")
}

func TestLoadDefaultFileNotFound(t *testing.T) {
	t.Setenv("SAP_CONFIG_FILE", "/nonexistent/path/systems.json")
	_, err := sapmcpconfig.LoadDefault()
	require.Error(t, err)
}
