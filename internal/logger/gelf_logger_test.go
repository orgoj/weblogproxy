package logger

import (
	"testing"

	"github.com/orgoj/weblogproxy/internal/config"
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
