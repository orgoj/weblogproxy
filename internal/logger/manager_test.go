package logger

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/orgoj/weblogproxy/internal/config"
)

// Reusing helper from file_logger_test.go
func tempLogFilePathManager(t *testing.T, pattern string) string {
	t.Helper()
	tmpDir := t.TempDir()
	return filepath.Join(tmpDir, pattern)
}

func TestNewManager_InitLoggers(t *testing.T) {
	tests := []struct {
		name                string
		destCfgs            []config.LogDestination
		expectInitError     bool
		expectedLoggerCount int
		expectedLoggers     map[string]string // map[name]type
	}{
		{
			name:                "No destinations",
			destCfgs:            []config.LogDestination{},
			expectInitError:     false,
			expectedLoggerCount: 0,
			expectedLoggers:     map[string]string{},
		},
		{
			name: "One valid file logger (no rotation)",
			destCfgs: []config.LogDestination{
				{
					Name:    "file1",
					Type:    "file",
					Enabled: true,
					Path:    tempLogFilePathManager(t, "file1.log"),
					Format:  "json",
				},
			},
			expectInitError:     false,
			expectedLoggerCount: 1,
			expectedLoggers:     map[string]string{"file1": "*logger.FileLogger"},
		},
		{
			name: "One valid file logger (with rotation)",
			destCfgs: []config.LogDestination{
				{
					Name:     "file_rotated",
					Type:     "file",
					Enabled:  true,
					Path:     tempLogFilePathManager(t, "file_rotated.log"),
					Format:   "text",
					Rotation: config.LogRotation{MaxSize: "1MB", MaxAge: "1d", MaxBackups: 1},
				},
			},
			expectInitError:     false,
			expectedLoggerCount: 1,
			expectedLoggers:     map[string]string{"file_rotated": "*logger.FileLogger"},
		},
		{
			name: "Mix of valid and disabled loggers",
			destCfgs: []config.LogDestination{
				{
					Name:    "valid_file",
					Type:    "file",
					Enabled: true,
					Path:    tempLogFilePathManager(t, "valid.log"),
					Format:  "json",
				},
				{
					Name:    "disabled_file",
					Type:    "file",
					Enabled: false, // Disabled
					Path:    tempLogFilePathManager(t, "disabled.log"),
					Format:  "text",
				},
			},
			expectInitError:     false,
			expectedLoggerCount: 1, // Only the enabled one
			expectedLoggers:     map[string]string{"valid_file": "*logger.FileLogger"},
		},
		{
			name: "Mix of valid and invalid (missing path) loggers",
			destCfgs: []config.LogDestination{
				{
					Name:    "valid_again",
					Type:    "file",
					Enabled: true,
					Path:    tempLogFilePathManager(t, "valid_again.log"),
					Format:  "text",
				},
				{
					Name:    "invalid_path",
					Type:    "file",
					Enabled: true,
					Path:    "", // Missing path
					Format:  "json",
				},
			},
			expectInitError:     true, // InitLoggers should return error if any logger fails
			expectedLoggerCount: 1,    // Check how many are initialized before error? Assume 1 for now.
			expectedLoggers:     map[string]string{"valid_again": "*logger.FileLogger"},
		},
		{
			name: "Invalid type logger",
			destCfgs: []config.LogDestination{
				{
					Name:    "unknown_type",
					Type:    "email", // Invalid type
					Enabled: true,
					Path:    "should_be_ignored",
				},
			},
			expectInitError:     true, // InitLoggers should return error
			expectedLoggerCount: 0,
			expectedLoggers:     map[string]string{},
		},
		// Note: Duplicate name validation should ideally be caught by config validation earlier.
		// If config validation passes duplicates, NewManager might overwrite or error.
		// Let's assume config validation prevents duplicates for this test.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture internal logs? Might be complex, skip for now.

			// 1. Create Manager (no args, no error expected)
			mgr := NewManager()
			if mgr == nil {
				t.Fatal("NewManager() returned nil manager")
			}
			// Ensure manager is closed eventually, even if InitLoggers fails partially
			defer mgr.CloseAll()

			// 2. Initialize Loggers
			initErr := mgr.InitLoggers(tt.destCfgs)

			// 3. Check for expected initialization error
			if tt.expectInitError {
				if initErr == nil {
					t.Errorf("InitLoggers() expected an error, but got nil")
				}
				// Check state *after* potential partial initialization if needed
				// For now, just check the error and return

				// Verify logger count after error - should be as expected before the error occurred?
				// Let's check the final state matching expected count for simplicity.
				mgr.mu.RLock()
				finalCount := len(mgr.loggers)
				mgr.mu.RUnlock()
				if finalCount != tt.expectedLoggerCount {
					t.Errorf("After expected error, expected %d loggers, but found %d", tt.expectedLoggerCount, finalCount)
				}
				// Could also check which specific loggers *were* created before error
				return
			}
			// --- If no initialization error expected ---
			if initErr != nil {
				t.Fatalf("InitLoggers() did not expect an error, but got: %v", initErr)
			}

			// Check logger count
			mgr.mu.RLock()
			count := len(mgr.loggers)
			mgr.mu.RUnlock()
			if count != tt.expectedLoggerCount {
				t.Errorf("Expected %d loggers, but found %d", tt.expectedLoggerCount, count)
			}

			// Check specific loggers and their types
			mgr.mu.RLock()
			for name, expectedType := range tt.expectedLoggers {
				lgr, exists := mgr.loggers[name]
				if !exists {
					t.Errorf("Expected logger with name '%s' not found", name)
				}
				if lgr == nil {
					t.Errorf("Logger with name '%s' is nil in map", name)
					continue
				}
				actualType := reflect.TypeOf(lgr).String()
				if actualType != expectedType {
					t.Errorf("Logger '%s' has wrong type: expected %s, got %s", name, expectedType, actualType)
				}
			}
			// Check that no unexpected loggers were created
			for name := range mgr.loggers {
				if _, ok := tt.expectedLoggers[name]; !ok {
					t.Errorf("Unexpected logger found in manager: '%s'", name)
				}
			}
			mgr.mu.RUnlock()
		})
	}
}

// TODO: TestManager_GetLogger
// TODO: TestManager_GetAllEnabledLoggerNames
// TODO: TestManager_CloseAll
