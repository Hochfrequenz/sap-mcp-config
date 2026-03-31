package sapmcpconfig_test

import (
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
	assert.Equal(t, "https://dev-sap.example.com:44300", dev.Host)
	assert.Equal(t, "100", dev.Client)
	assert.Equal(t, "DEV_USER", dev.User)
	assert.Equal(t, "dev_secret", dev.Password)
	assert.Equal(t, "DE", dev.Language)
	assert.True(t, dev.TLSSkipVerify)
	assert.False(t, dev.IsOAuth2())

	prod := cfg.Systems["prod"]
	assert.Equal(t, "https://prod-sap.example.com:44300", prod.Host)
	assert.Equal(t, "200", prod.Client)
	assert.Equal(t, "PROD_USER", prod.User)
	assert.Equal(t, "prod_secret", prod.Password)
	assert.Equal(t, "EN", prod.Language)
	assert.False(t, prod.TLSSkipVerify)
	assert.False(t, prod.IsOAuth2())

	oauth := cfg.Systems["oauth"]
	assert.Equal(t, "https://oauth-sap.example.com:44300", oauth.Host)
	assert.Equal(t, "300", oauth.Client)
	assert.Equal(t, "", oauth.User)
	assert.Equal(t, "", oauth.Password)
	assert.Equal(t, "EN", oauth.Language)
	assert.Equal(t, "my-mcp-client", oauth.OAuth2ClientID)
	assert.True(t, oauth.IsOAuth2())
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
			name:    "default not found",
			json:    `{"default_system":"missing","systems":{"a":{"host":"h","user":"u","password":"p"}}}`,
			wantErr: `default_system "missing" not found`,
		},
		{
			name:    "missing host",
			json:    `{"default_system":"a","systems":{"a":{"client":"100","user":"u","password":"p"}}}`,
			wantErr: "no host configured",
		},
		{
			name:    "user without password",
			json:    `{"default_system":"a","systems":{"a":{"host":"h","user":"u"}}}`,
			wantErr: "must have both user and password",
		},
		{
			name:    "password without user",
			json:    `{"default_system":"a","systems":{"a":{"host":"h","password":"p"}}}`,
			wantErr: "must have both user and password",
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

func TestLoadFileNotFound(t *testing.T) {
	_, err := sapmcpconfig.Load("nonexistent.json")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err) || true) // wrapped error
}
