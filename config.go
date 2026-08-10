// Package sapmcpconfig provides shared configuration types for MCP servers
// that connect to SAP systems.
//
// Use [Load] or [LoadDefault] to read a configuration file (JSON or YAML).
// Use [Parse] to parse JSON bytes directly, or [ParseYAML] for YAML.
// All functions validate the configuration before returning it.
package sapmcpconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the fallback location when SAP_CONFIG_FILE is not set.
const DefaultConfigPath = "~/.config/sap-mcp/systems.json"

// envPlaceholder matches an ${env:VAR} placeholder. VAR must be a plain
// identifier, so anything else — e.g. ${env:not an identifier} — stays a
// literal and can never be mistaken for a placeholder.
//
// Deliberately not anchored with ^/$: a placeholder may sit anywhere inside a
// larger value, and a value may hold several — "https://${env:HOST}:${env:PORT}"
// must resolve both. Anchoring would restrict placeholders to whole-value use
// only and silently break that. The identifier character class is what keeps
// matches tight, not the position.
var envPlaceholder = regexp.MustCompile(`\$\{env:(?P<var>[A-Za-z_][A-Za-z0-9_]*)\}`)

// envPlaceholderVar is the index of envPlaceholder's named "var" group, so
// matches are read by name rather than by a bare literal index.
var envPlaceholderVar = envPlaceholder.SubexpIndex("var")

// fieldStatus records what interpolation did to a single field.
type fieldStatus struct {
	// fromEnv is true when any part of the value came from a placeholder.
	fromEnv bool
	// unusable is true when a placeholder could not be turned into a usable
	// value, so the field has no meaningful content to validate.
	unusable bool
}

// topLevelKey identifies fields that belong to the config itself rather than to
// a system. The NUL makes a collision vanishingly unlikely rather than
// impossible — a system literally named "\x00config" would still match — but it
// does keep a system named "" from being mistaken for a top-level field, which
// was the case that actually occurred.
const topLevelKey = "\x00config"

// placeholderReport is the outcome of interpolating one config: the errors to
// report, plus per-field status so validation can skip fields that have no
// usable value and avoid echoing values that came from the environment.
//
// status is keyed by system name and then field name; topLevelKey holds the
// default_system entry.
type placeholderReport struct {
	messages []string
	status   map[string]map[string]fieldStatus
}

func (r placeholderReport) get(system, field string) fieldStatus {
	return r.status[system][field]
}

// unusable reports whether field has no usable value, because a placeholder in
// it could not be resolved.
func (r placeholderReport) unusable(system, field string) bool {
	return r.get(system, field).unusable
}

// describe renders value for an error message, withholding it when it came from
// the environment — an env-supplied host or client may itself be sensitive, and
// the user did not write it in the file, so echoing it back helps nobody.
func (r placeholderReport) describe(system, field, value string) string {
	if r.get(system, field).fromEnv {
		return "the value taken from the environment"
	}
	return fmt.Sprintf("%q", value)
}

func (r *placeholderReport) addf(system, field, format string, args ...any) {
	detail := fmt.Sprintf(format, args...)
	if system == topLevelKey {
		r.messages = append(r.messages, field+" "+detail)
		return
	}
	r.messages = append(r.messages, fmt.Sprintf("system %q: %s %s", system, field, detail))
}

func (r *placeholderReport) setStatus(system, field string, st fieldStatus) {
	if r.status[system] == nil {
		r.status[system] = map[string]fieldStatus{}
	}
	r.status[system][field] = st
}

// resolve replaces every ${env:VAR} in value and records what happened.
//
// Substitution is single-pass: replacement text is never rescanned, so a secret
// whose value happens to contain ${env:...} cannot be used to read another
// variable.
//
// Detection happens here rather than by re-scanning the finished value, because
// only at this point do we still know the placeholder came from the config
// document. Re-scanning afterwards would misread a secret that legitimately
// contains "${env:...}" as an unresolved placeholder — and would build the error
// message out of that secret's plaintext.
func (r *placeholderReport) resolve(system, field, value string) string {
	var st fieldStatus
	resolved := envPlaceholder.ReplaceAllStringFunc(value, func(match string) string {
		name := envPlaceholder.FindStringSubmatch(match)[envPlaceholderVar]
		st.fromEnv = true
		env, ok := os.LookupEnv(name)
		switch {
		case !ok:
			st.unusable = true
			r.addf(system, field, "references ${env:%s}, which is not set in the environment", name)
			return match
		case env == "":
			// Defined-but-empty is the shape an unpopulated CI secret takes, and
			// letting it through would silently strip a credential — for user and
			// password that even flips the system to OAuth2.
			st.unusable = true
			r.addf(system, field, "references ${env:%s}, which is set but empty", name)
			return env
		}
		return env
	})
	if st != (fieldStatus{}) {
		r.setStatus(system, field, st)
	}
	return resolved
}

// interpolateSystems resolves ${env:VAR} placeholders in every string field of
// cfg, in place, and reports what could not be resolved.
//
// Interpolating the already-decoded struct — rather than a generic document —
// is deliberate: it keeps each value exactly as the YAML/JSON decoder produced
// it. Re-encoding a generic document would let YAML re-tag unquoted scalars, so
// a password of 007 would silently load as 7.
func interpolateSystems(cfg *Config) placeholderReport {
	report := placeholderReport{status: map[string]map[string]fieldStatus{}}
	cfg.DefaultSystem = report.resolve(topLevelKey, "default_system", cfg.DefaultSystem)
	// Sorted, so the reported errors come out in a stable order. Ranging over the
	// map directly makes the message order vary between runs, which Python (whose
	// dicts keep insertion order) never does.
	for _, name := range sortedSystemNames(cfg.Systems) {
		sys := cfg.Systems[name]
		sys.ConnectionName = report.resolve(name, "connection_name", sys.ConnectionName)
		sys.Host = report.resolve(name, "host", sys.Host)
		sys.Client = report.resolve(name, "client", sys.Client)
		sys.User = report.resolve(name, "user", sys.User)
		sys.Password = report.resolve(name, "password", sys.Password)
		sys.Language = report.resolve(name, "language", sys.Language)
		sys.OAuth2ClientID = report.resolve(name, "oauth2_client_id", sys.OAuth2ClientID)
		cfg.Systems[name] = sys
	}
	return report
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
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config (expected YAML): %w", err)
	}
	return normalizeAndValidate(&cfg)
}

// Parse unmarshals JSON bytes into a Config and validates it.
// ${env:VAR} placeholders are resolved from the environment first.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config (expected JSON): %w", err)
	}
	return normalizeAndValidate(&cfg)
}

// normalizeAndValidate interpolates, validates and normalizes a parsed Config.
func normalizeAndValidate(cfg *Config) (*Config, error) {
	report := interpolateSystems(cfg)
	if err := cfg.validate(report); err != nil {
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
	return c.validate(placeholderReport{})
}

// validate checks the Config, taking into account which fields could not be
// resolved from the environment. A zero report means no interpolation happened,
// which is what [Config.Validate] passes for a hand-built Config.
func (c *Config) validate(report placeholderReport) error {
	// Placeholder problems are reported alongside everything else so the user
	// still fixes the whole file in one pass.
	errs := append([]string(nil), report.messages...)
	if len(c.Systems) == 0 {
		return fmt.Errorf("config has no systems defined")
	}
	if !report.unusable(topLevelKey, "default_system") {
		if _, ok := c.Systems[c.DefaultSystem]; !ok {
			errs = append(errs, fmt.Sprintf("default_system %s not found in systems",
				report.describe(topLevelKey, "default_system", c.DefaultSystem)))
		}
	}
	for _, name := range sortedSystemNames(c.Systems) {
		sys := c.Systems[name]
		// A field whose placeholder could not be resolved has no meaningful value,
		// so its remaining checks are skipped — reporting `host must start with
		// http://` on top of the unset-variable error would just be noise.
		// Crucially this also suppresses the both-or-neither check, which would
		// otherwise read an unresolved user and password as a deliberate OAuth2
		// system.
		if !report.unusable(name, "host") {
			if sys.Host == "" {
				errs = append(errs, fmt.Sprintf("system %q: host is required", name))
			} else if !strings.HasPrefix(sys.Host, "http://") && !strings.HasPrefix(sys.Host, "https://") {
				errs = append(errs, fmt.Sprintf("system %q: host must start with http:// or https://, got %s",
					name, report.describe(name, "host", sys.Host)))
			}
		}
		if !report.unusable(name, "client") && sys.Client != "" && (len(sys.Client) != 3 || !isDigits(sys.Client)) {
			errs = append(errs, fmt.Sprintf("system %q: client must be a 3-digit string (e.g. \"100\"), got %s",
				name, report.describe(name, "client", sys.Client)))
		}
		if !report.unusable(name, "user") && !report.unusable(name, "password") &&
			(sys.User == "") != (sys.Password == "") {
			errs = append(errs, fmt.Sprintf("system %q: must have both user and password, or neither (for OAuth2)", name))
		}
		if !report.unusable(name, "language") && sys.Language != "" {
			// Report the normalized value, matching Python, whose BeforeValidator has
			// already uppercased the field by the time it reaches validation.
			lang := strings.ToUpper(sys.Language)
			if lang != "DE" && lang != "EN" {
				errs = append(errs, fmt.Sprintf("system %q: language must be \"DE\" or \"EN\", got %s",
					name, report.describe(name, "language", lang)))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// sortedSystemNames returns the system names in a stable order, so error
// messages do not shuffle between runs.
func sortedSystemNames(systems map[string]SAPSystem) []string {
	names := make([]string, 0, len(systems))
	for name := range systems {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
