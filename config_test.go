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

// TestParseRejectsTrailingData guards the behaviour json.Unmarshal gave us for
// free before Parse decoded by hand. Decoder.More() alone would let the two
// closing-bracket cases through.
func TestParseRejectsTrailingData(t *testing.T) {
	const valid = `{"default_system":"s","systems":{"s":{"host":"https://x:443","client":"100","user":"u","password":"p"}}}`

	for _, trailing := range []string{" true", "}", "]", ", {}", "garbage", `{"b":2}`} {
		t.Run(trailing, func(t *testing.T) {
			_, err := sapmcpconfig.Parse([]byte(valid + trailing))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unexpected data after top-level value")
		})
	}

	t.Run("trailing whitespace is fine", func(t *testing.T) {
		cfg, err := sapmcpconfig.Parse([]byte(valid + "\n  \n"))
		require.NoError(t, err)
		assert.Len(t, cfg.Systems, 1)
	})
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
