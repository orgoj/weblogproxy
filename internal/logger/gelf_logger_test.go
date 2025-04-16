package logger

import (
	"strings"
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

// mockGelfWriter is a mock gelf.Writer for testing
type mockGelfWriter struct {
	lastMessage *gelf.Message
	writeCalled bool
	closeCalled bool
	returnError error // Optional error to return from WriteMessage
}

func (m *mockGelfWriter) WriteMessage(msg *gelf.Message) error {
	m.writeCalled = true
	m.lastMessage = msg
	return m.returnError
}

// Write implements io.Writer (needed if gelf.Writer implicitly satisfies it somewhere)
func (m *mockGelfWriter) Write(p []byte) (n int, err error) {
	// This mock doesn't need to implement io.Writer functionality for this test
	return len(p), nil
}

func (m *mockGelfWriter) Close() error {
	m.closeCalled = true
	return nil
}

func TestGelfTruncation(t *testing.T) {
	// Override writer factories for this test
	origUDPWriterFactory := gelfUDPWriterFactory
	origTCPWriterFactory := gelfTCPWriterFactory
	defer func() {
		gelfUDPWriterFactory = origUDPWriterFactory
		gelfTCPWriterFactory = origTCPWriterFactory
	}()

	mockWriter := &mockGelfWriter{}
	gelfUDPWriterFactory = func(addr string) (*gelf.UDPWriter, error) {
		// Return a dummy writer, we'll replace it later
		return &gelf.UDPWriter{}, nil
	}
	gelfTCPWriterFactory = func(addr string) (*gelf.TCPWriter, error) {
		// Return a dummy writer, we'll replace it later
		return &gelf.TCPWriter{}, nil
	}

	// Define test cases
	tests := []struct {
		name             string
		config           config.LogDestination
		record           map[string]interface{}
		expectedShortLen int
		expectedFullLen  int
		expectedShortEnd string
		expectedFullEnd  string
	}{
		{
			name: "No truncation (UDP default size)",
			config: config.LogDestination{
				Name: "udp-default", Type: "gelf", Host: "localhost", Port: 12201,
			},
			record: map[string]interface{}{
				"message":      "short message",
				"full_message": "this is a longer full message",
			},
			expectedShortLen: 13, // len("short message")
			expectedFullLen:  29, // len("this is a longer full message")
		},
		{
			name: "No truncation (TCP unlimited)",
			config: config.LogDestination{
				Name: "tcp-unlimited", Type: "gelf", Host: "localhost", Port: 12201, Protocol: "tcp",
			},
			record: map[string]interface{}{
				"message":      "short message tcp",
				"full_message": "this is a longer full message for tcp",
			},
			expectedShortLen: 17, // len("short message tcp")
			expectedFullLen:  37, // len("this is a longer full message for tcp")
		},
		{
			name: "Truncate Short only (exceeds available)",
			config: config.LogDestination{
				Name: "trunc-short", Type: "gelf", Host: "localhost", Port: 12201, MaxMessageSize: 1080, // Available = 1080 - 1024 = 56
			},
			record: map[string]interface{}{
				"message":      "this is a very long short message that will certainly exceed the available limit",
				"full_message": "this full message should be cleared",
			},
			expectedShortLen: 56,
			expectedFullLen:  0, // Full message cleared
			expectedShortEnd: "...truncated",
		},
		{
			name: "Truncate Full only (Short fits, Full exceeds remaining)",
			config: config.LogDestination{
				Name: "trunc-full", Type: "gelf", Host: "localhost", Port: 12201, MaxMessageSize: 1100, // Available = 1100 - 1024 = 76
			},
			record: map[string]interface{}{
				"message":      "short message fits",                                                                      // len=18
				"full_message": "this very long full message will need truncation because it exceeds the remaining space", // Remaining = 76 - 18 = 58
			},
			expectedShortLen: 18,
			expectedFullLen:  58,
			expectedFullEnd:  "...truncated",
		},
		{
			name: "Truncate Short and Full (very small limit)",
			config: config.LogDestination{
				Name: "trunc-both", Type: "gelf", Host: "localhost", Port: 12201, MaxMessageSize: 1050, // Available = 1050 - 1024 = 26
			},
			record: map[string]interface{}{
				"message":      "this short message is too long",
				"full_message": "this full message is also too long",
			},
			expectedShortLen: 26,
			expectedFullLen:  0,
			expectedShortEnd: "...truncated",
		},
		{
			name: "Truncate Short (not enough space for ellipsis)",
			config: config.LogDestination{
				Name: "trunc-no-ellipsis", Type: "gelf", Host: "localhost", Port: 12201, MaxMessageSize: 1030, // Available = 1030 - 1024 = 6
			},
			record: map[string]interface{}{
				"message":      "this short message is too long",
				"full_message": "this full message is also too long",
			},
			expectedShortLen: 6, // Just cut
			expectedFullLen:  0,
			expectedShortEnd: "this s", // Expected ending after cutting
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWriter.lastMessage = nil // Reset mock writer
			mockWriter.writeCalled = false

			logger, err := NewGelfLogger(tt.config)
			if err != nil {
				t.Fatalf("NewGelfLogger() error = %v", err)
			}
			logger.writer = mockWriter // Replace the actual writer with the mock

			err = logger.Log(tt.record)
			if err != nil {
				t.Fatalf("logger.Log() error = %v", err)
			}

			if !mockWriter.writeCalled {
				t.Fatal("WriteMessage was not called")
			}

			if mockWriter.lastMessage == nil {
				t.Fatal("Last message is nil")
			}

			// Check lengths
			if len(mockWriter.lastMessage.Short) != tt.expectedShortLen {
				t.Errorf("Expected Short length %d, got %d (value: %q)", tt.expectedShortLen, len(mockWriter.lastMessage.Short), mockWriter.lastMessage.Short)
			}
			if len(mockWriter.lastMessage.Full) != tt.expectedFullLen {
				t.Errorf("Expected Full length %d, got %d (value: %q)", tt.expectedFullLen, len(mockWriter.lastMessage.Full), mockWriter.lastMessage.Full)
			}

			// Check endings if truncation happened
			if tt.expectedShortEnd != "" && !strings.HasSuffix(mockWriter.lastMessage.Short, tt.expectedShortEnd) {
				t.Errorf("Expected Short to end with %q, got %q", tt.expectedShortEnd, mockWriter.lastMessage.Short)
			}
			if tt.expectedFullEnd != "" && !strings.HasSuffix(mockWriter.lastMessage.Full, tt.expectedFullEnd) {
				t.Errorf("Expected Full to end with %q, got %q", tt.expectedFullEnd, mockWriter.lastMessage.Full)
			}

			// logger.Close() // Clean up
			if err := logger.Close(); err != nil {
				t.Errorf("logger.Close() returned an unexpected error: %v", err)
			}
		})
	}
}
