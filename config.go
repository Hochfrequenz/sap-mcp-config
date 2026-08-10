// Package sapmcpconfig provides shared configuration types for MCP servers
// that connect to SAP systems.
//
// Use [Load] or [LoadDefault] to read a configuration file (JSON or YAML).
// Use [Parse] to parse JSON bytes directly, or [ParseYAML] for YAML.
// All functions validate the configuration before returning it.
package sapmcpconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the fallback location when SAP_CONFIG_FILE is not set.
const DefaultConfigPath = "~/.config/sap-mcp/systems.json"

// envPlaceholder matches an ${env:VAR} placeholder. VAR must be a plain
// identifier, so anything else — e.g. ${env:not an identifier} — stays a
// literal and can never be mistaken for a placeholder.
var envPlaceholder = regexp.MustCompile(`\$\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolateEnv replaces ${env:VAR} with the environment value in every string
// of a decoded document.
//
// Substitution is single-pass: replacement text is never rescanned, so a secret
// whose value happens to contain ${env:...} cannot be used to read another
// variable.
//
// Placeholders whose variable is unset are left verbatim; [Config.Validate]
// turns those into a collected configuration error rather than silently
// yielding an empty string.
func interpolateEnv(v any) any {
	switch t := v.(type) {
	case string:
		return envPlaceholder.ReplaceAllStringFunc(t, func(match string) string {
			name := envPlaceholder.FindStringSubmatch(match)[1]
			if value, ok := os.LookupEnv(name); ok {
				return value
			}
			return match
		})
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, item := range t {
			out[key] = interpolateEnv(item)
		}
		return out
	case map[any]any:
		out := make(map[any]any, len(t))
		for key, item := range t {
			out[key] = interpolateEnv(item)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = interpolateEnv(item)
		}
		return out
	default:
		return v
	}
}

// unresolvedEnvVar returns the name of the first ${env:VAR} in value whose
// variable is unset.
//
// Only a genuinely unset variable counts. Because interpolation is single-pass,
// a placeholder that survives into a resolved value must have come from the
// environment itself — and if that variable is set, the text is a literal the
// user meant to keep, not a mistake.
func unresolvedEnvVar(value string) (string, bool) {
	for _, m := range envPlaceholder.FindAllStringSubmatch(value, -1) {
		if _, ok := os.LookupEnv(m[1]); !ok {
			return m[1], true
		}
	}
	return "", false
}

// Supported config file extensions for automatic format detection in [Load].
var yamlExtensions = map[string]bool{".yaml": true, ".yml": true}

// SAPSystem describes a single SAP system's connection details and credentials.
//
// Always obtain instances through [Load], [LoadDefault], or [Parse] to ensure
// all fields are validated.
type SAPSystem struct {
	ConnectionName string `json:"connection_name,omitempty" yaml:"connection_name,omitempty"`
	Host           string `json:"host" yaml:"host"`
	Client         string `json:"client" yaml:"client"`
	User           string `json:"user,omitempty" yaml:"user,omitempty"`
	Password       string `json:"password,omitempty" yaml:"password,omitempty"`
	Language       string `json:"language,omitempty" yaml:"language,omitempty"`
	TLSSkipVerify  bool   `json:"tls_skip_verify,omitempty" yaml:"tls_skip_verify,omitempty"`
	OAuth2ClientID string `json:"oauth2_client_id,omitempty" yaml:"oauth2_client_id,omitempty"`
}

// IsOAuth2 returns true when the system is configured for OAuth2 (no user/password).
func (s SAPSystem) IsOAuth2() bool {
	return s.User == "" && s.Password == ""
}

// String returns a human-readable representation with the password masked.
// This prevents accidental credential leaks in logs or error messages.
func (s SAPSystem) String() string {
	pwd := ""
	if s.Password != "" {
		pwd = "***"
	}
	prefix := ""
	if s.ConnectionName != "" {
		prefix = "ConnectionName:" + s.ConnectionName + " "
	}
	return fmt.Sprintf("SAPSystem{%sHost:%s Client:%s User:%s Password:%s Language:%s}", prefix, s.Host, s.Client, s.User, pwd, s.Language)
}

// Format implements fmt.Formatter to ensure the password is masked for all
// format verbs including %+v and %#v.
func (s SAPSystem) Format(f fmt.State, verb rune) {
	// Always delegate to String() so the password is never printed.
	_, _ = fmt.Fprint(f, s.String())
}

// Config holds all configured SAP systems and a default system name.
type Config struct {
	DefaultSystem string               `json:"default_system" yaml:"default_system"`
	Systems       map[string]SAPSystem `json:"systems" yaml:"systems"`
}

// GetDefault returns a copy of the default system's configuration.
func (c *Config) GetDefault() SAPSystem {
	return c.Systems[c.DefaultSystem]
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// Load reads a Config from a JSON or YAML file and validates it.
// The format is detected by file extension: .yaml/.yml for YAML, everything
// else (including .json) for JSON.
// The path may start with ~ which is expanded to the user's home directory.
func Load(path string) (*Config, error) {
	expanded := expandHome(path)
	data, err := os.ReadFile(expanded)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}
	if yamlExtensions[strings.ToLower(filepath.Ext(expanded))] {
		return ParseYAML(data)
	}
	return Parse(data)
}

// LoadDefault loads the configuration from the path specified in the
// SAP_CONFIG_FILE environment variable, falling back to [DefaultConfigPath].
// It loads .env files from the current directory before reading the
// environment variable.
func LoadDefault() (*Config, error) {
	_ = godotenv.Load() // best-effort; missing .env is fine
	path := os.Getenv("SAP_CONFIG_FILE")
	if path == "" {
		path = DefaultConfigPath
	}
	return Load(path)
}

// ParseYAML unmarshals YAML bytes into a Config and validates it.
// ${env:VAR} placeholders are resolved from the environment first.
func ParseYAML(data []byte) (*Config, error) {
	// Decode into a generic document so interpolation sees the same string
	// values the Python implementation does, then re-encode and decode into
	// Config. Interpolating the raw file bytes instead would corrupt any secret
	// containing a quote or backslash.
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing config (expected YAML): %w", err)
	}
	resolved, err := yaml.Marshal(interpolateEnv(doc))
	if err != nil {
		return nil, fmt.Errorf("parsing config (expected YAML): %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config (expected YAML): %w", err)
	}
	return normalizeAndValidate(&cfg)
}

// Parse unmarshals JSON bytes into a Config and validates it.
// ${env:VAR} placeholders are resolved from the environment first.
func Parse(data []byte) (*Config, error) {
	// See ParseYAML for why this decodes into a generic document first.
	var doc any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep numbers byte-exact across the re-encode
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parsing config (expected JSON): %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parsing config (expected JSON): unexpected data after top-level value")
	}
	resolved, err := json.Marshal(interpolateEnv(doc))
	if err != nil {
		return nil, fmt.Errorf("parsing config (expected JSON): %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(resolved, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config (expected JSON): %w", err)
	}
	return normalizeAndValidate(&cfg)
}

// normalizeAndValidate validates and normalizes a parsed Config.
func normalizeAndValidate(cfg *Config) (*Config, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	for name, sys := range cfg.Systems {
		sys.Language = strings.ToUpper(sys.Language)
		if sys.Language == "" {
			sys.Language = "EN"
		}
		cfg.Systems[name] = sys
	}
	return cfg, nil
}

// Validate checks that the Config is well-formed.
// It collects all errors so users can fix everything in one pass.
func (c *Config) Validate() error {
	var errs []string
	if len(c.Systems) == 0 {
		return fmt.Errorf("config has no systems defined")
	}
	if v, ok := unresolvedEnvVar(c.DefaultSystem); ok {
		errs = append(errs, fmt.Sprintf("default_system references ${env:%s}, which is not set in the environment", v))
	} else if _, ok := c.Systems[c.DefaultSystem]; !ok {
		errs = append(errs, fmt.Sprintf("default_system %q not found in systems", c.DefaultSystem))
	}
	for name, sys := range c.Systems {
		// A field still holding a placeholder has no meaningful value yet, so its
		// remaining checks are skipped — reporting `host must start with http://,
		// got "${env:SAP_HOST}"` on top of the unset-variable error would just be
		// noise. Crucially this also suppresses the both-or-neither check, which
		// would otherwise read an unresolved user and password as a deliberate
		// OAuth2 system.
		unresolvedErrs, unresolved := collectUnresolved(name, sys)
		errs = append(errs, unresolvedErrs...)
		if !unresolved["host"] {
			if sys.Host == "" {
				errs = append(errs, fmt.Sprintf("system %q: host is required", name))
			} else if !strings.HasPrefix(sys.Host, "http://") && !strings.HasPrefix(sys.Host, "https://") {
				errs = append(errs, fmt.Sprintf("system %q: host must start with http:// or https://, got %q", name, sys.Host))
			}
		}
		if !unresolved["client"] && sys.Client != "" && (len(sys.Client) != 3 || !isDigits(sys.Client)) {
			errs = append(errs, fmt.Sprintf("system %q: client must be a 3-digit string (e.g. \"100\"), got %q", name, sys.Client))
		}
		if !unresolved["user"] && !unresolved["password"] && (sys.User == "") != (sys.Password == "") {
			errs = append(errs, fmt.Sprintf("system %q: must have both user and password, or neither (for OAuth2)", name))
		}
		if sys.Language != "" {
			lang := strings.ToUpper(sys.Language)
			if lang != "DE" && lang != "EN" {
				errs = append(errs, fmt.Sprintf("system %q: language must be \"DE\" or \"EN\", got %q", name, sys.Language))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// collectUnresolved appends an error for every field of sys still holding a
// placeholder, and returns the affected field names so the caller can skip
// their remaining checks.
//
// Language is deliberately absent: an unresolved placeholder there is already
// rejected by the existing DE/EN check, and in Python by the Literal field
// type — reporting it here would make the two implementations disagree.
//
// Only the variable name is reported, never the value it resolved to, so a
// validation failure cannot echo a credential.
func collectUnresolved(name string, sys SAPSystem) ([]string, map[string]bool) {
	fields := []struct{ field, value string }{
		{"connection_name", sys.ConnectionName},
		{"host", sys.Host},
		{"client", sys.Client},
		{"user", sys.User},
		{"password", sys.Password},
		{"oauth2_client_id", sys.OAuth2ClientID},
	}
	var errs []string
	unresolved := make(map[string]bool)
	for _, f := range fields {
		if v, ok := unresolvedEnvVar(f.value); ok {
			unresolved[f.field] = true
			errs = append(errs, fmt.Sprintf("system %q: %s references ${env:%s}, which is not set in the environment", name, f.field, v))
		}
	}
	return errs, unresolved
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
