package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Helper function to create a temporary config file
func createTempConfigFile(t *testing.T, content string) string {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "config.yaml")
	err := os.WriteFile(tempFile, []byte(content), 0644)
	require.NoError(t, err, "Failed to create temporary config file")
	return tempFile
}

func TestLoadConfig_Valid(t *testing.T) {
	// Load the main test config file from the root directory
	cfg, err := LoadConfig("../../config/test.yaml")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// --- Assertions for specific values from test.yaml ---

	// Server
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8081, cfg.Server.Port)
	assert.Equal(t, "embedded", cfg.Server.Mode)
	assert.Equal(t, "__wlp_test__", cfg.Server.PathPrefix)
	assert.True(t, cfg.Server.CORS.Enabled)
	assert.Contains(t, cfg.Server.CORS.AllowedOrigins, "*")
	assert.Equal(t, "public, max-age=10, stale-while-revalidate=5", cfg.Server.Headers["Cache-Control"])
	assert.Equal(t, 20480, cfg.Server.RequestLimits.MaxBodySize)
	assert.Equal(t, 5000, cfg.Server.RequestLimits.RateLimit)
	assert.Equal(t, []string{"127.0.0.1"}, cfg.Server.TrustedProxies)
	assert.Equal(t, 200, cfg.Server.UnknownRoute.Code)
	assert.Equal(t, "public, max-age=3600", cfg.Server.UnknownRoute.CacheControl)

	// Security
	assert.Equal(t, "super-secret-test-key-!@#$", cfg.Security.Token.Secret)
	assert.Equal(t, "30m", cfg.Security.Token.Expiration)

	// Log Destinations
	require.Len(t, cfg.LogDestinations, 2, "Expected 2 log destinations")

	// Dest 1: file_rotated
	dest1 := cfg.LogDestinations[0]
	assert.Equal(t, "file_rotated", dest1.Name)
	assert.Equal(t, "file", dest1.Type)
	assert.True(t, dest1.Enabled)
	assert.Equal(t, "json", dest1.Format)
	assert.Equal(t, "/tmp/weblogproxy-test-rotation.log", dest1.Path)
	assert.Equal(t, 2, len(dest1.AddLogData))
	assert.Equal(t, "output_format", dest1.AddLogData[0].Name)
	assert.Equal(t, "static", dest1.AddLogData[0].Source)
	assert.Equal(t, "json_lines", dest1.AddLogData[0].Value)
	assert.Equal(t, "dest_specific_header_val", dest1.AddLogData[1].Name)
	assert.Equal(t, "header", dest1.AddLogData[1].Source)
	assert.Equal(t, "X-Custom-Dest-Header", dest1.AddLogData[1].Value)
	assert.Equal(t, 3, dest1.Rotation.MaxBackups)
	assert.False(t, dest1.Rotation.Compress)
	assert.Equal(t, "1d", dest1.Rotation.MaxAge)
	assert.Equal(t, "1", dest1.Rotation.MaxSize)

	// Dest 2: file_plain
	dest2 := cfg.LogDestinations[1]
	assert.Equal(t, "file_plain", dest2.Name)
	assert.Equal(t, "file", dest2.Type)
	assert.True(t, dest2.Enabled)
	assert.Equal(t, "/tmp/weblogproxy-test-plain.log", dest2.Path)
	assert.Equal(t, "text", dest2.Format)
	require.Len(t, dest2.AddLogData, 1)
	assert.Equal(t, "output_format", dest2.AddLogData[0].Name)

	// Log Config (Rules)
	require.Len(t, cfg.LogConfig, 7, "Expected 7 log rules")

	// Rule 0
	rule0 := cfg.LogConfig[0]
	assert.True(t, rule0.Enabled)
	assert.True(t, rule0.Continue)
	require.Len(t, rule0.AddLogData, 3)
	assert.Equal(t, "server_hostname", rule0.AddLogData[0].Name)
	assert.Equal(t, "static", rule0.AddLogData[0].Source)
	require.Len(t, rule0.ScriptInjection, 1)
	assert.Equal(t, "https://test.example.com/scripts/base-tracking.js", rule0.ScriptInjection[0].URL)
	assert.True(t, rule0.ScriptInjection[0].Async)
	assert.False(t, rule0.ScriptInjection[0].Defer)

	// Rule 1
	rule1 := cfg.LogConfig[1]
	assert.Equal(t, "test-site-1", rule1.Condition.SiteID)
	assert.Contains(t, rule1.Condition.GTMIDs, "gtm-test-A")
	assert.Contains(t, rule1.Condition.IPs, "192.168.1.0/24")
	assert.True(t, rule1.Enabled)
	assert.False(t, rule1.Continue) // Default value
	assert.Equal(t, []string{"file_rotated", "file_plain"}, rule1.LogDestinations)
	require.Len(t, rule1.AddLogData, 4)
	assert.Equal(t, "campaign", rule1.AddLogData[0].Name)
	assert.Equal(t, "query", rule1.AddLogData[0].Source)
	assert.Equal(t, "utm_campaign", rule1.AddLogData[0].Value)
	assert.Equal(t, "request_id", rule1.AddLogData[3].Name)
	assert.Equal(t, "header", rule1.AddLogData[3].Source)
	assert.Equal(t, "X-Rule1-Request-ID", rule1.AddLogData[3].Value)
	require.Len(t, rule1.ScriptInjection, 3)
	assert.Equal(t, "https://cdn.example.com/tracker.js", rule1.ScriptInjection[0].URL)
	assert.True(t, rule1.ScriptInjection[0].Async)
	assert.False(t, rule1.ScriptInjection[0].Defer)
	assert.Equal(t, "https://test.example.com/local/scripts/init.js", rule1.ScriptInjection[1].URL)
	assert.False(t, rule1.ScriptInjection[1].Async)
	assert.True(t, rule1.ScriptInjection[1].Defer)
	assert.Equal(t, "https://test.example.com/scripts/base-tracking.js", rule1.ScriptInjection[2].URL)
	assert.True(t, rule1.ScriptInjection[2].Async)
	assert.False(t, rule1.ScriptInjection[2].Defer)

	// Poslední pravidlo (Disabled)
	lastRule := cfg.LogConfig[6]
	assert.Equal(t, "disabled-site", lastRule.Condition.SiteID)
	assert.False(t, lastRule.Enabled)
}

func TestLoadConfig_InvalidCases(t *testing.T) {
	testCases := []struct {
		name          string
		config        string
		expectedError string
	}{
		{
			name: "Missing token secret",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: ""
    expiration: "24h"
`,
			expectedError: "token.secret cannot be empty",
		},
		{
			name: "Invalid token expiration format",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test_secret"
    expiration: "invalid"
`,
			expectedError: "invalid security.token.expiration",
		},
		{
			name: "Zero token expiration",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test_secret"
    expiration: "0s"
`,
			expectedError: "duration must be positive: '0s'",
		},
		{
			name: "Invalid server mode",
			config: `
server:
  mode: "invalid_mode"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test_secret"
    expiration: "24h"
`,
			expectedError: "invalid server.mode",
		},
		{
			name: "Missing path prefix in embedded mode",
			config: `
server:
  mode: "embedded"
  path_prefix: ""
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
`,
			expectedError: "path_prefix is required when server.mode is 'embedded'",
		},
		{
			name: "Missing domain in standalone mode",
			config: `
server:
  mode: "standalone"
  domain: ""
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
`,
			expectedError: "domain is required when server.mode is 'standalone'",
		},
		{
			name: "Duplicate destination name",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "dup_name"
    type: file
    path: /tmp/log1.log
    format: json
  - name: "dup_name"
    type: file
    path: /tmp/log2.log
    format: json
`,
			expectedError: "duplicate name 'dup_name' found",
		},
		{
			name: "Missing destination name",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: ""
    type: file
    path: /tmp/log.log
    format: json
`,
			expectedError: "log_destinations[0]: name is required",
		},
		{
			name: "Unknown destination type",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "mydest"
    type: "unknown"
`,
			expectedError: "log_destinations[mydest]: unknown type 'unknown'",
		},
		{
			name: "Missing path for file destination",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "file_dest"
    type: file
    path: ""
    format: json
`,
			expectedError: "log_destinations[file_dest]: path is required for type 'file'",
		},
		{
			name: "Invalid format for file destination",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "file_dest"
    type: file
    path: "/tmp/log.log"
    format: "xml"
`,
			expectedError: "log_destinations[file_dest]: invalid format 'xml'",
		},
		{
			name: "Missing host for GELF destination",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "gelf_dest"
    type: gelf
    host: ""
    port: 12201
`,
			expectedError: "log_destinations[gelf_dest]: host is required for type 'gelf'",
		},
		{
			name: "Invalid GELF protocol",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "gelf_dest"
    type: gelf
    host: "graylog.example.com"
    port: 12201
    protocol: "http"
`,
			expectedError: "log_destinations[gelf_dest]: invalid protocol 'http'",
		},
		{
			name: "Invalid GELF compression",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "gelf_dest"
    type: gelf
    host: "graylog.example.com"
    port: 12201
    compression_type: "zip"
`,
			expectedError: "log_destinations[gelf_dest]: invalid compression_type 'zip'",
		},
		{
			name: "Invalid AddLogData source",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - condition: {}
    enabled: true
    add_log_data:
      - name: "test_field"
        source: "database"
        value: "some_value"
`,
			expectedError: "invalid source 'database'",
		},
		{
			name: "Missing name in AddLogData",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - condition: {}
    enabled: true
    add_log_data:
      - name: ""
        source: "static"
        value: "some_value"
`,
			expectedError: "log_config[0].add_log_data[0]: name is required",
		},
		{
			name: "Missing value for static AddLogData",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - condition: {}
    enabled: true
    add_log_data:
      - name: "missing_value"
        source: "static"
        value: ""
`,
			expectedError: "value is required for source 'static'",
		},
		{
			name: "Rule references non-existent destination",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "real_dest"
    type: file
    path: "/tmp/real.log"
    format: json
log_config:
  - condition: {}
    enabled: true
    log_destinations: ["real_dest", "fake_dest"]
`,
			expectedError: "specified log_destination 'fake_dest' not found",
		},
		{
			name: "Negative rotation value",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "file_dest"
    type: file
    path: "/tmp/log.log"
    format: json
    rotation:
      max_size: "-5M"
`,
			expectedError: "size cannot be negative: -5",
		},
		{
			name: "Missing_path_prefix_in_embedded_mode",
			config: `
server:
  mode: "embedded"
  path_prefix: ""
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
`,
			expectedError: "path_prefix is required when server.mode is 'embedded'",
		},
		{
			name: "Missing_domain_in_standalone_mode",
			config: `
server:
  mode: "standalone"
  domain: ""
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
`,
			expectedError: "domain is required when server.mode is 'standalone'",
		},
		{
			name: "Invalid domain in standalone mode",
			config: `
server:
  mode: "standalone"
  domain: "invalid_domain!"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
`,
			expectedError: "server.domain 'invalid_domain!' is not a valid domain name",
		},
		{
			name: "Invalid add_log_data source",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "file1"
    type: file
    path: /tmp/log.log
    format: json
    add_log_data:
      - name: "foo"
        source: "invalid"
        value: "bar"
`,
			expectedError: "log_destinations[file1][0]: invalid source 'invalid'",
		},
		{
			name: "Empty add_log_data name",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_destinations:
  - name: "file1"
    type: file
    path: /tmp/log.log
    format: json
    add_log_data:
      - name: ""
        source: "static"
        value: "bar"
`,
			expectedError: "log_destinations[file1][0]: name is required",
		},
		{
			name: "Invalid script_injection URL",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - enabled: true
    script_injection:
      - url: "ftp://not-allowed.com/script.js"
        async: true
`,
			expectedError: "log_config[0].script_injection[0]: url 'ftp://not-allowed.com/script.js' is not a valid URL",
		},
		{
			name: "Invalid header name in rule condition",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - enabled: true
    condition:
      headers:
        "Invalid Header!": "value"
`,
			expectedError: "log_config[0].condition.headers: header name 'Invalid Header!' is not valid",
		},
		{
			name: "Invalid header value type in rule condition",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - enabled: true
    condition:
      headers:
        X-Test: 123
`,
			expectedError: "log_config[0].condition.headers: header 'X-Test' value must be string or bool, got int",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configFile := createTempConfigFile(t, tc.config)
			_, err := LoadConfig(configFile)
			require.Error(t, err, "Expected an error when loading invalid config")
			assert.Contains(t, err.Error(), tc.expectedError, "Error message mismatch")
		})
	}
}

// Added: Tests for ParseDuration
func TestParseDuration(t *testing.T) {
	tests := []struct {
		name        string
		durationStr string
		expected    time.Duration
		wantErr     bool
	}{
		{"valid minutes", "10m", 10 * time.Minute, false},
		{"valid hours", "2h", 2 * time.Hour, false},
		{"valid mixed", "1h30m", 90 * time.Minute, false},
		{"valid seconds", "45s", 45 * time.Second, false},
		{"valid complex", "2h15m30s", (2*time.Hour + 15*time.Minute + 30*time.Second), false},
		{"zero duration", "0s", 0, true},
		{"zero duration no unit", "0", 0, true}, // time.ParseDuration fails
		{"negative duration", "-5m", 0, true},
		{"invalid format", "10minutes", 0, true},
		{"invalid format space", "10 m", 0, true},
		{"empty string", "", 0, true},
		{"no unit", "10", 0, true}, // time.ParseDuration fails
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.durationStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Added: Tests for ParseSize
func TestParseSize(t *testing.T) {
	tests := []struct {
		name     string
		sizeStr  string
		expected int64
		wantErr  bool
	}{
		{"valid bytes no suffix", "1024", 1024, false},
		{"valid kilobytes K", "10K", 10 * 1024, false},
		{"valid kilobytes KB", "2KB", 2 * 1024, false},
		{"valid megabytes M", "5M", 5 * 1024 * 1024, false},
		{"valid megabytes MB", "100MB", 100 * 1024 * 1024, false},
		{"valid gigabytes G", "1G", 1 * 1024 * 1024 * 1024, false},
		{"valid gigabytes GB", "2GB", 2 * 1024 * 1024 * 1024, false},
		{"valid lowercase k", "5k", 5 * 1024, false},
		{"valid lowercase mb", "50mb", 50 * 1024 * 1024, false},
		{"valid with space", " 100 MB ", 100 * 1024 * 1024, false},
		{"zero bytes", "0", 0, false},
		{"zero kilobytes", "0k", 0, false},
		{"invalid number", "abcM", 0, true},
		{"invalid suffix", "10X", 0, true},
		{"negative number", "-5M", 0, true},
		{"empty string", "", 0, true},
		{"suffix only", "MB", 0, true},
		{"overflow G large number", "9000000000G", 0, true},
		{"max int64 bytes", "9223372036854775807", 9223372036854775807, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSize(tt.sizeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseSize() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TODO: Add tests for validateConfig (more comprehensive)

func TestLoadConfig_Headers_And_FieldRemoval(t *testing.T) {
	testCases := []struct {
		name          string
		config        string
		expectedError string
	}{
		{
			name: "Headers in condition - valid",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - condition:
      headers:
        Content-Type: "application/json"
        Authorization: true
        X-Debug-Mode: false
    enabled: true
`,
			expectedError: "",
		},
		{
			name: "Field removal with false value in add_log_data",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
security:
  token:
    secret: "test"
    expiration: "24h"
log_config:
  - condition: {}
    enabled: true
    add_log_data:
      - name: "field_to_remove"
        source: "static"
        value: "false" 
`,
			expectedError: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configFile := createTempConfigFile(t, tc.config)
			result, err := LoadConfig(configFile)

			if tc.expectedError != "" {
				require.Error(t, err, "Expected an error when loading config")
				assert.Contains(t, err.Error(), tc.expectedError, "Error message mismatch")
			} else {
				require.NoError(t, err, "Expected no error when loading valid config")
				require.NotNil(t, result, "Config should be loaded")
			}
		})
	}
}

func TestValidateConfig_CORS(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid CORS config",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: true
    allowed_origins:
      - "https://example.com"
      - "http://localhost:3000"
    max_age: 3600
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError: false,
		},
		{
			name: "CORS disabled - valid",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: false
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError: false,
		},
		{
			name: "Empty allowed_origins",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: true
    allowed_origins: []
    max_age: 3600
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError:   true,
			errorContains: "server.cors.allowed_origins cannot be empty when CORS is enabled",
		},
		{
			name: "Invalid origin format",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: true
    allowed_origins:
      - "example.com"
    max_age: 3600
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError:   true,
			errorContains: "must start with 'http://' or 'https://'",
		},
		{
			name: "Wildcard origin is valid",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: true
    allowed_origins:
      - "*"
    max_age: 3600
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError: false,
		},
		{
			name: "Negative max_age",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: true
    allowed_origins:
      - "https://example.com"
    max_age: -1
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError:   true,
			errorContains: "server.cors.max_age cannot be negative",
		},
		{
			name: "CORS disabled with invalid settings is valid (not validated)",
			config: `
server:
  mode: "standalone"
  domain: "example.com"
  unknown_route:
    code: 200
    cache_control: "public, max-age=3600"
  cors:
    enabled: false
    allowed_origins: []
    max_age: -1
security:
  token:
    secret: "test-secret"
    expiration: "1h"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We use yaml unmarshalling directly instead of LoadConfig to avoid validation
			var cfg Config
			// Setting default values (same as in LoadConfig)
			cfg.Server.Host = "0.0.0.0"
			cfg.Server.Port = 8080
			cfg.Server.Mode = "standalone"
			cfg.Server.Protocol = "http"
			cfg.Server.JavaScript.GlobalObjectName = "wlp"

			err := yaml.Unmarshal([]byte(tt.config), &cfg)
			require.NoError(t, err, "YAML unmarshalling should not fail")

			// Now manually perform validation
			err = validateConfig(&cfg)

			if tt.expectError {
				assert.Error(t, err, "Expected validation error")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "Expected specific error message")
				}
			} else {
				assert.NoError(t, err, "Validation should not fail")
			}
		})
	}
}
