package config

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

// AddLogDataSpec defines how to add or modify a field in the log record.
type AddLogDataSpec struct {
	Name   string `yaml:"name"`
	Source string `yaml:"source"` // static, header, query, post
	Value  string `yaml:"value"`  // Static value or key/name for header/query/post
}

// ScriptInjectionSpec defines a script to be injected.
type ScriptInjectionSpec struct {
	URL   string `yaml:"url"`
	Async bool   `yaml:"async,omitempty"` // Default: true?
	Defer bool   `yaml:"defer,omitempty"` // Default: false?
}

// LogRotation defines parameters for log file rotation.
type LogRotation struct {
	MaxSize    string `yaml:"max_size,omitempty"`    // e.g., "100MB", "50k"
	MaxAge     string `yaml:"max_age,omitempty"`     // e.g., "7d", "2w", "1m"
	MaxBackups int    `yaml:"max_backups,omitempty"` // Still int
	Compress   bool   `yaml:"compress,omitempty"`
}

// Config represents the application configuration
type Config struct {
	ConfigReload struct {
		Enabled  bool `yaml:"enabled"`
		Interval int  `yaml:"interval"` // seconds
	} `yaml:"config_reload"`

	AppLog struct {
		Level          string `yaml:"level"`
		ShowHealthLogs bool   `yaml:"show_health_logs"`
	} `yaml:"app_log"`

	Server struct {
		Host             string   `yaml:"host"`
		Port             int      `yaml:"port"`
		Mode             string   `yaml:"mode"`     // standalone or embedded
		Protocol         string   `yaml:"protocol"` // http or https, used for generating URLs
		Domain           string   `yaml:"domain"`   // Full domain name for standalone mode
		PathPrefix       string   `yaml:"path_prefix"`
		TrustedProxies   []string `yaml:"trusted_proxies"`
		HealthAllowedIPs []string `yaml:"health_allowed_ips"`
		CORS             struct {
			Enabled        bool     `yaml:"enabled"`
			AllowedOrigins []string `yaml:"allowed_origins"`
			MaxAge         int      `yaml:"max_age"` // seconds
		} `yaml:"cors"`
		Headers       map[string]string `yaml:"headers"`
		RequestLimits struct {
			MaxBodySize int `yaml:"max_body_size"` // bytes
			RateLimit   int `yaml:"rate_limit"`    // requests per minute (TODO: Implement)
		} `yaml:"request_limits"`
		JavaScript struct {
			GlobalObjectName string `yaml:"global_object_name"` // Name of the global JS object (default: wlp)
		} `yaml:"javascript"`
		UnknownRoute struct {
			Code         int    `yaml:"code"`
			CacheControl string `yaml:"cache_control"`
		} `yaml:"unknown_route"`
		ClientIPHeader string `yaml:"client_ip_header"` // Header to use for real client IP (e.g. CF-Connecting-IP, X-Real-IP)
	} `yaml:"server"`

	Security struct {
		Token struct {
			Secret     string `yaml:"secret"`
			Expiration string `yaml:"expiration"` // Changed to string, e.g. "10m", "1h"
		} `yaml:"token"`
		// RequestLimits moved to Server section
	} `yaml:"security"`

	LogDestinations []LogDestination `yaml:"log_destinations"`
	LogConfig       []LogRule        `yaml:"log_config"`
}

// LogDestination represents a logging destination configuration
type LogDestination struct {
	Name    string `yaml:"name"` // Mandatory, unique identifier
	Type    string `yaml:"type"` // Mandatory: file, gelf
	Enabled bool   `yaml:"enabled"`

	MaxMessageSize int `yaml:"max_message_size,omitempty"` // Optional: Max message size in bytes (default depends on type: file=4096, gelf-udp=8192, gelf-tcp=unlimited)

	// File specific
	Path     string      `yaml:"path,omitempty"`     // Mandatory for type: file
	Format   string      `yaml:"format,omitempty"`   // Mandatory for type: file (json or text)
	Rotation LogRotation `yaml:"rotation,omitempty"` // Use exported type

	// GELF specific
	Host            string `yaml:"host,omitempty"`             // Mandatory for type: gelf
	Port            int    `yaml:"port,omitempty"`             // Mandatory for type: gelf
	Protocol        string `yaml:"protocol,omitempty"`         // Optional for type: gelf (udp or tcp, default udp)
	CompressionType string `yaml:"compression_type,omitempty"` // Optional for type: gelf (gzip, zlib, none, default none)

	AddLogData []AddLogDataSpec `yaml:"add_log_data,omitempty"`
}

// LogRuleCondition specifies criteria for matching requests.
type LogRuleCondition struct {
	SiteID     string                 `yaml:"site_id,omitempty"`
	GTMIDs     []string               `yaml:"gtm_ids,omitempty"`
	UserAgents []string               `yaml:"user_agents,omitempty"`
	IPs        []string               `yaml:"ips,omitempty"`
	Headers    map[string]interface{} `yaml:"headers,omitempty"` // Header name and value (string or false for removal)
}

// LogRule represents a logging rule configuration
type LogRule struct {
	Condition          LogRuleCondition      `yaml:"condition"` // Use named type
	Enabled            bool                  `yaml:"enabled"`
	Continue           bool                  `yaml:"continue,omitempty"`             // Default: false
	LogScriptDownloads bool                  `yaml:"log_script_downloads,omitempty"` // If true and continue:true, accumulates script download logging; if true and continue:false, enables script download logging
	ScriptInjection    []ScriptInjectionSpec `yaml:"script_injection,omitempty"`
	AddLogData         []AddLogDataSpec      `yaml:"add_log_data,omitempty"`
	LogDestinations    []string              `yaml:"log_destinations,omitempty"` // Optional list of destination names
	JavaScriptOptions  struct {
		TrackURL       bool `yaml:"track_url,omitempty"`
		TrackTraceback bool `yaml:"track_traceback,omitempty"`
	} `yaml:"javascript_options,omitempty"`
}

// LoadConfig reads the configuration file from the given path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Config path is provided by user via command-line flag, considered trusted input.
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg Config
	// Default values can be set here before unmarshalling if needed
	cfg.Server.Host = "0.0.0.0"                    // Default host
	cfg.Server.Port = 8080                         // Default port
	cfg.Server.Mode = "standalone"                 // Default mode
	cfg.Server.Protocol = "http"                   // Default protocol
	cfg.Server.JavaScript.GlobalObjectName = "wlp" // Default JS global object name
	cfg.AppLog.Level = "WARN"                      // Default application log level
	cfg.AppLog.ShowHealthLogs = false              // Default health logs setting
	cfg.Server.UnknownRoute.Code = 200
	cfg.Server.UnknownRoute.CacheControl = "public, max-age=3600"

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error parsing config file '%s': %w", path, err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &cfg, nil
}

// validateConfig performs semantic validation of the configuration
func validateConfig(cfg *Config) error {
	// Basic security checks
	if cfg.Security.Token.Secret == "" {
		return errors.New("security.token.secret cannot be empty")
	}
	// Token expiration validation
	_, err := ParseDuration(cfg.Security.Token.Expiration)
	if err != nil {
		return fmt.Errorf("invalid security.token.expiration: %w", err)
	}

	// AppLog validation
	if cfg.AppLog.Level != "" {
		validLevels := []string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}
		levelValid := false
		uppercaseLevel := strings.ToUpper(cfg.AppLog.Level)

		for _, level := range validLevels {
			if uppercaseLevel == level {
				levelValid = true
				break
			}
		}

		if !levelValid {
			return fmt.Errorf("app_log.level '%s' is invalid, must be one of: TRACE, DEBUG, INFO, WARN, ERROR, FATAL", cfg.AppLog.Level)
		}
	}

	// Server validation
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server.port: %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "standalone" && cfg.Server.Mode != "embedded" {
		return fmt.Errorf("invalid server.mode: '%s', must be 'standalone' or 'embedded'", cfg.Server.Mode)
	}
	if cfg.Server.Protocol != "http" && cfg.Server.Protocol != "https" {
		return fmt.Errorf("invalid server.protocol: '%s', must be 'http' or 'https'", cfg.Server.Protocol)
	}
	// Validate JavaScript global object name
	if cfg.Server.JavaScript.GlobalObjectName == "" {
		return errors.New("server.javascript.global_object_name cannot be empty")
	}
	if !isValidJSIdentifier(cfg.Server.JavaScript.GlobalObjectName) {
		return fmt.Errorf("server.javascript.global_object_name '%s' is not a valid JavaScript identifier", cfg.Server.JavaScript.GlobalObjectName)
	}

	if cfg.Server.Mode == "embedded" && cfg.Server.PathPrefix == "" {
		return errors.New("server.path_prefix is required when server.mode is 'embedded'")
	}
	if cfg.Server.Mode == "standalone" && cfg.Server.Domain == "" {
		return errors.New("server.domain is required when server.mode is 'standalone'")
	}
	if cfg.Server.Mode == "standalone" && !isValidDomain(cfg.Server.Domain) {
		return fmt.Errorf("server.domain '%s' is not a valid domain name", cfg.Server.Domain)
	}
	if cfg.Server.RequestLimits.MaxBodySize < 0 {
		return errors.New("server.request_limits.max_body_size cannot be negative")
	}

	// CORS validation
	if cfg.Server.CORS.Enabled {
		if len(cfg.Server.CORS.AllowedOrigins) == 0 {
			return errors.New("server.cors.allowed_origins cannot be empty when CORS is enabled")
		}

		// Validate each allowed origin
		for i, origin := range cfg.Server.CORS.AllowedOrigins {
			if origin == "*" {
				// Wildcard is valid (but potentially dangerous)
				continue
			}

			// Check that origin has a valid format (should start with http:// or https://)
			if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
				return fmt.Errorf("server.cors.allowed_origins[%d]: origin '%s' must start with 'http://' or 'https://'", i, origin)
			}
		}

		// Validate MaxAge
		if cfg.Server.CORS.MaxAge < 0 {
			return errors.New("server.cors.max_age cannot be negative")
		}
	}

	// TODO: Add validation for RateLimit when implemented

	// Log Destinations validation
	destinationNames := make(map[string]bool)
	for i, dest := range cfg.LogDestinations {
		if dest.Name == "" {
			return fmt.Errorf("log_destinations[%d]: name is required", i)
		}
		if destinationNames[dest.Name] {
			return fmt.Errorf("log_destinations: duplicate name '%s' found", dest.Name)
		}
		destinationNames[dest.Name] = true

		switch dest.Type {
		case "file":
			if dest.Path == "" {
				return fmt.Errorf("log_destinations[%s]: path is required for type 'file'", dest.Name)
			}
			if dest.Format != "json" && dest.Format != "text" {
				return fmt.Errorf("log_destinations[%s]: invalid format '%s', must be 'json' or 'text' for type 'file'", dest.Name, dest.Format)
			}
			// Validation for rotation params
			if dest.Rotation.MaxSize != "" { // Validate only if set
				_, err := ParseSize(dest.Rotation.MaxSize)
				if err != nil {
					return fmt.Errorf("log_destinations[%s]: invalid rotation.max_size: %w", dest.Name, err)
				}
			}
			if dest.Rotation.MaxAge != "" { // Validate only if set
				_, err := ParseDuration(dest.Rotation.MaxAge) // Use existing ParseDuration
				if err != nil {
					return fmt.Errorf("log_destinations[%s]: invalid rotation.max_age: %w", dest.Name, err)
				}
			}
			if dest.Rotation.MaxBackups < 0 {
				return fmt.Errorf("log_destinations[%s]: rotation.max_backups cannot be negative", dest.Name)
			}
		case "gelf":
			if dest.Host == "" {
				return fmt.Errorf("log_destinations[%s]: host is required for type 'gelf'", dest.Name)
			}
			if dest.Port <= 0 || dest.Port > 65535 {
				return fmt.Errorf("log_destinations[%s]: invalid port %d for type 'gelf'", dest.Name, dest.Port)
			}
			if dest.Protocol != "" && dest.Protocol != "udp" && dest.Protocol != "tcp" {
				return fmt.Errorf("log_destinations[%s]: invalid protocol '%s', must be 'udp' or 'tcp' for type 'gelf'", dest.Name, dest.Protocol)
			}
			// Set default GELF protocol if empty
			if dest.Protocol == "" {
				cfg.LogDestinations[i].Protocol = "udp" // Assign back to the slice element
			}
			if dest.CompressionType != "" && dest.CompressionType != "gzip" && dest.CompressionType != "zlib" && dest.CompressionType != "none" {
				return fmt.Errorf("log_destinations[%s]: invalid compression_type '%s', must be 'gzip', 'zlib', or 'none' for type 'gelf'", dest.Name, dest.CompressionType)
			}
			// Set default GELF compression if empty
			if dest.CompressionType == "" {
				cfg.LogDestinations[i].CompressionType = "none" // Assign back to the slice element
			}
		default:
			return fmt.Errorf("log_destinations[%s]: unknown type '%s'", dest.Name, dest.Type)
		}

		// Validate AddLogData for the destination
		if err := validateAddLogDataSpecs(dest.AddLogData, fmt.Sprintf("log_destinations[%s]", dest.Name)); err != nil {
			return err
		}
	}

	// Log Rules validation
	for i, rule := range cfg.LogConfig {
		rulePath := fmt.Sprintf("log_config[%d]", i)
		// Validate AddLogData within rule
		if err := validateAddLogDataSpecs(rule.AddLogData, rulePath+".add_log_data"); err != nil {
			return err
		}
		// Validate ScriptInjection URLs
		for j, script := range rule.ScriptInjection {
			if script.URL == "" {
				return fmt.Errorf("%s.script_injection[%d]: url is required", rulePath, j)
			}
			if !isValidURL(script.URL) {
				return fmt.Errorf("%s.script_injection[%d]: url '%s' is not a valid URL", rulePath, j, script.URL)
			}
		}
		// Validate that specified log_destinations exist
		for _, destName := range rule.LogDestinations {
			if !destinationNames[destName] {
				return fmt.Errorf("%s: specified log_destination '%s' not found in top-level log_destinations", rulePath, destName)
			}
		}
		// Validate headers (must be string or bool)
		for k, v := range rule.Condition.Headers {
			if !isValidHeaderName(k) {
				return fmt.Errorf("%s.condition.headers: header name '%s' is not valid", rulePath, k)
			}
			switch v := v.(type) {
			case string:
				// ok
			case bool:
				// ok
			default:
				return fmt.Errorf("%s.condition.headers: header '%s' value must be string or bool, got %T", rulePath, k, v)
			}
		}
	}

	if cfg.Server.UnknownRoute.Code < 100 || cfg.Server.UnknownRoute.Code > 599 {
		return fmt.Errorf("server.unknown_route.code must be a valid HTTP status code (100-599), got %d", cfg.Server.UnknownRoute.Code)
	}
	if cfg.Server.UnknownRoute.CacheControl == "" {
		return errors.New("server.unknown_route.cache_control cannot be empty")
	}

	return nil
}

// validateAddLogDataSpecs validates a slice of AddLogDataSpec
func validateAddLogDataSpecs(specs []AddLogDataSpec, path string) error {
	validSources := map[string]bool{"static": true, "header": true, "query": true, "post": true}
	for j, spec := range specs {
		specPath := fmt.Sprintf("%s[%d]", path, j)
		if spec.Name == "" {
			return fmt.Errorf("%s: name is required", specPath)
		}
		if !validSources[spec.Source] {
			return fmt.Errorf("%s: invalid source '%s', must be one of %v", specPath, spec.Source, getMapKeys(validSources))
		}
		// Value can be empty for header/query/post (means get the value of that key)
		// Value is mandatory for static source? Yes.
		if spec.Source == "static" && spec.Value == "" {
			return fmt.Errorf("%s: value is required for source 'static'", specPath)
		}
	}
	return nil
}

// Helper function to get keys from a map[string]bool
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ValidateConfig uses go-playground/validator for struct-level validation.
// It complements the semantic validation in validateConfig.
func ValidateConfig(cfg *Config) error {
	validate := validator.New()

	// Register custom validators if needed
	// err := validate.RegisterValidation("cidr_or_ip", validateCIDROrIP)
	// if err != nil {
	// 	return fmt.Errorf("failed to register custom validator: %w", err)
	// }

	// Validate the main config struct
	err := validate.Struct(cfg)
	if err != nil {
		// Translate validation errors into a more readable format
		var validationErrors []string
		for _, err := range err.(validator.ValidationErrors) {
			// Customize error messages based on tag and field
			fieldName := err.Field()
			tag := err.Tag()
			message := fmt.Sprintf("Field validation for '%s' failed on the '%s' tag", fieldName, tag)
			// Add more specific messages here if needed
			validationErrors = append(validationErrors, message)
		}
		return errors.New(strings.Join(validationErrors, "; "))
	}

	// Perform additional semantic validation (that validator can't easily handle)
	if err := validateConfig(cfg); err != nil {
		return err // Return the semantic validation error
	}

	return nil
}

// ParseDuration parses a duration string (e.g., "10m", "1h30m", "7d").
// Supports standard time.ParseDuration units plus 'd' for days.
// Returns an error if the format is invalid or the duration is non-positive.
func ParseDuration(durationStr string) (time.Duration, error) {
	durationStr = strings.TrimSpace(durationStr)
	if durationStr == "" {
		return 0, errors.New("duration string cannot be empty")
	}

	// Handle 'd' suffix manually
	if strings.HasSuffix(strings.ToLower(durationStr), "d") {
		numStr := strings.TrimSuffix(strings.ToLower(durationStr), "d")
		days, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number format for days in '%s': %w", durationStr, err)
		}
		if days < 0 {
			return 0, fmt.Errorf("duration (days) cannot be negative: %d", days)
		}
		d := time.Duration(days) * 24 * time.Hour
		if d <= 0 && days > 0 { // Handle potential overflow if days is huge, though unlikely
			return 0, fmt.Errorf("duration %dd results in overflow or zero duration", days)
		} else if d <= 0 && days == 0 { // Check for zero explicitly
			return 0, fmt.Errorf("duration must be positive: '%s'", durationStr)
		}
		return d, nil
	}

	// Use standard time.ParseDuration for other units
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format '%s': %w", durationStr, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive: '%s'", durationStr)
	}
	return d, nil
}

// ParseSize parses a size string (e.g., "10MB", "5k", "1G") into bytes.
// Supports K, M, G suffixes (case-insensitive).
// TODO: Limit support to K, M, G (and KB, MB, GB) suffixes only. Larger units are unlikely for logs.
func ParseSize(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(strings.ToUpper(sizeStr))
	if sizeStr == "" {
		return 0, errors.New("size string cannot be empty")
	}

	var multiplier int64 = 1
	suffix := ""

	if strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		suffix = "KB"
	} else if strings.HasSuffix(sizeStr, "K") {
		multiplier = 1024
		suffix = "K"
	} else if strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		suffix = "MB"
	} else if strings.HasSuffix(sizeStr, "M") {
		multiplier = 1024 * 1024
		suffix = "M"
	} else if strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		suffix = "GB"
	} else if strings.HasSuffix(sizeStr, "G") {
		multiplier = 1024 * 1024 * 1024
		suffix = "G"
	} // END OF SUPPORTED UNITS

	numStr := sizeStr
	if suffix != "" {
		numStr = strings.TrimSuffix(sizeStr, suffix)
	}
	numStr = strings.TrimSpace(numStr)

	// Use big.Int for invalid format detection and negative numbers
	numBig := new(big.Int)
	_, ok := numBig.SetString(numStr, 10)
	if !ok {
		return 0, fmt.Errorf("invalid number format in size string '%s'", sizeStr)
	}

	if numBig.Sign() < 0 {
		return 0, fmt.Errorf("size cannot be negative: %s", numBig.String())
	}
	if numBig.Sign() == 0 {
		return 0, nil // Zero is valid
	}

	// Multiply using big.Int
	multiplierBig := big.NewInt(multiplier)
	resultBig := new(big.Int).Mul(numBig, multiplierBig)

	// Check for int64 overflow
	maxInt64 := big.NewInt(1<<63 - 1)
	if resultBig.Cmp(maxInt64) > 0 {
		return 0, fmt.Errorf("size value %s%s results in overflow (exceeds max int64)", numBig.String(), suffix)
	}

	// Safely convert to int64
	// Check if the result can be represented as int64
	// (Cmp should cover this, but to be sure)
	if !resultBig.IsInt64() {
		return 0, fmt.Errorf("size value %s%s cannot be represented as int64", numBig.String(), suffix)
	}

	return resultBig.Int64(), nil
}

// isValidJSIdentifier checks if a string is a valid JavaScript identifier
func isValidJSIdentifier(s string) bool {
	if s == "" {
		return false
	}
	// Check first character (must be letter, underscore, or dollar sign)
	firstChar := s[0]
	if ('a' > firstChar || firstChar > 'z') &&
		('A' > firstChar || firstChar > 'Z') &&
		firstChar != '_' && firstChar != '$' {
		return false
	}

	// Check rest of the characters (can also include digits)
	for i := 1; i < len(s); i++ {
		c := s[i]
		if ('a' > c || c > 'z') &&
			('A' > c || c > 'Z') &&
			('0' > c || c > '9') &&
			c != '_' && c != '$' {
			return false
		}
	}

	// Check if it's not a reserved word
	reservedWords := map[string]bool{
		"break": true, "case": true, "catch": true, "class": true, "const": true,
		"continue": true, "debugger": true, "default": true, "delete": true,
		"do": true, "else": true, "export": true, "extends": true, "false": true,
		"finally": true, "for": true, "function": true, "if": true, "import": true,
		"in": true, "instanceof": true, "new": true, "null": true, "return": true,
		"super": true, "switch": true, "this": true, "throw": true, "true": true,
		"try": true, "typeof": true, "var": true, "void": true, "while": true,
		"with": true, "yield": true, "let": true, "static": true, "enum": true,
		"await": true, "implements": true, "package": true, "protected": true,
		"interface": true, "private": true, "public": true,
	}

	return !reservedWords[s]
}

// Helper: validace domény
func isValidDomain(domain string) bool {
	if len(domain) < 1 || len(domain) > 253 {
		return false
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-') {
				return false
			}
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

// Helper: validace URL (pouze základní kontrola schématu a domény)
func isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "/")
}

// Helper: validace hlavičky
func isValidHeaderName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
