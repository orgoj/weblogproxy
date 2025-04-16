package logger

import (
	"testing"

	"github.com/orgoj/weblogproxy/internal/config"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

func TestGelfLogger_Name(t *testing.T) {
	logger := &GelfLogger{
		name: "test-gelf",
	}

	if logger.Name() != "test-gelf" {
		t.Errorf("Expected name to be 'test-gelf', got '%s'", logger.Name())
	}
}

func TestNewGelfLogger_ValidationErrors(t *testing.T) {
	// Test missing host
	cfg := config.LogDestination{
		Name: "test-gelf",
		Type: "gelf",
		Port: 12201,
	}

	_, err := NewGelfLogger(cfg)
	if err == nil {
		t.Error("Expected error for missing host, got nil")
	}

	// Test invalid port
	cfg = config.LogDestination{
		Name: "test-gelf",
		Type: "gelf",
		Host: "localhost",
		Port: 0,
	}

	_, err = NewGelfLogger(cfg)
	if err == nil {
		t.Error("Expected error for invalid port, got nil")
	}
}

func TestGetLevel(t *testing.T) {
	tests := []struct {
		name     string
		record   map[string]interface{}
		expected int32
	}{
		{
			name:     "No level",
			record:   map[string]interface{}{},
			expected: 6, // Default INFO
		},
		{
			name:     "Integer level",
			record:   map[string]interface{}{"level": 3},
			expected: 3,
		},
		{
			name:     "Float level",
			record:   map[string]interface{}{"level": 4.0},
			expected: 4,
		},
		{
			name:     "String level - error",
			record:   map[string]interface{}{"level": "error"},
			expected: 3,
		},
		{
			name:     "String level - warning",
			record:   map[string]interface{}{"level": "warning"},
			expected: 4,
		},
		{
			name:     "String level - info",
			record:   map[string]interface{}{"level": "info"},
			expected: 6,
		},
		{
			name:     "String level - debug",
			record:   map[string]interface{}{"level": "debug"},
			expected: 7,
		},
		{
			name:     "Unknown string level",
			record:   map[string]interface{}{"level": "unknown"},
			expected: 6, // Default INFO
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := getLevel(tt.record)
			if level != tt.expected {
				t.Errorf("getLevel() = %v, want %v", level, tt.expected)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	tests := []struct {
		name         string
		record       map[string]interface{}
		key          string
		defaultValue string
		expected     string
	}{
		{
			name:         "Key exists as string",
			record:       map[string]interface{}{"message": "test message"},
			key:          "message",
			defaultValue: "default",
			expected:     "test message",
		},
		{
			name:         "Key exists as int",
			record:       map[string]interface{}{"code": 404},
			key:          "code",
			defaultValue: "unknown",
			expected:     "404",
		},
		{
			name:         "Key doesn't exist",
			record:       map[string]interface{}{},
			key:          "missing",
			defaultValue: "not found",
			expected:     "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.record, tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGelfCompression(t *testing.T) {
	origNewUDPWriter := gelfUDPWriterFactory
	origNewTCPWriter := gelfTCPWriterFactory
	origSetUDPCompression := setUDPCompression

	defer func() {
		// Restore original factories after the test
		gelfUDPWriterFactory = origNewUDPWriter
		gelfTCPWriterFactory = origNewTCPWriter
		setUDPCompression = origSetUDPCompression
	}()

	// Track what compression is used when setting UDPWriter.CompressionType
	var capturedCompressionType gelf.CompressType

	// Override the setUDPCompression function to capture the compression type
	setUDPCompression = func(writer *gelf.UDPWriter, compType gelf.CompressType) {
		capturedCompressionType = compType
		// We don't actually need to set it in the test
	}

	// Mock the UDPWriter factory
	gelfUDPWriterFactory = func(addr string) (*gelf.UDPWriter, error) {
		return &gelf.UDPWriter{}, nil
	}

	// Mock the TCPWriter factory
	gelfTCPWriterFactory = func(addr string) (*gelf.TCPWriter, error) {
		return &gelf.TCPWriter{}, nil
	}

	tests := []struct {
		name           string
		compressionCfg string
		expectedType   gelf.CompressType
		protocol       string
	}{
		{
			name:           "Gzip compression",
			compressionCfg: "gzip",
			expectedType:   gelf.CompressGzip,
			protocol:       "udp",
		},
		{
			name:           "Zlib compression",
			compressionCfg: "zlib",
			expectedType:   gelf.CompressZlib,
			protocol:       "udp",
		},
		{
			name:           "No compression",
			compressionCfg: "none",
			expectedType:   gelf.CompressNone,
			protocol:       "udp",
		},
		{
			name:           "Default compression (empty)",
			compressionCfg: "",
			expectedType:   gelf.CompressNone, // Default is none
			protocol:       "udp",
		},
		{
			name:           "TCP protocol (compression not used)",
			compressionCfg: "gzip",
			expectedType:   0, // Not set for TCP
			protocol:       "tcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset captured value between tests
			capturedCompressionType = 99 // Invalid value that won't match any real value

			// Create a test configuration
			cfg := config.LogDestination{
				Name:            "test-gelf",
				Type:            "gelf",
				Host:            "localhost",
				Port:            12201,
				Protocol:        tt.protocol,
				CompressionType: tt.compressionCfg,
			}

			// Try to create a logger - this should set the compression type
			_, err := NewGelfLogger(cfg)
			if err != nil {
				t.Fatalf("Failed to create GELF logger: %v", err)
			}

			// For UDP protocol, verify the compression type was set correctly
			if tt.protocol == "udp" && capturedCompressionType != tt.expectedType {
				t.Errorf("Expected compression type %v, got %v",
					tt.expectedType, capturedCompressionType)
			}
		})
	}
}
