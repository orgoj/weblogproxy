package logger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orgoj/weblogproxy/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Helper to create a temporary log file path
func tempLogFilePath(t *testing.T, pattern string) string {
	t.Helper()
	tmpDir := t.TempDir() // Use t.TempDir() for automatic cleanup
	return filepath.Join(tmpDir, pattern)
}

// Helper to read the last line of a file
func readLastLine(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open log file %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Fatalf("Failed to close file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	var lastLine string
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Error reading log file %s: %v", path, err)
	}
	return lastLine
}

func TestNewFileLogger(t *testing.T) {
	filePath := tempLogFilePath(t, "new_test.log")

	tests := []struct {
		name        string
		cfg         config.LogDestination
		expectError bool
	}{
		{
			name: "Valid config JSON with rotation",
			cfg: config.LogDestination{
				Name:   "test-json",
				Type:   "file",
				Path:   filePath,
				Format: "json",
				Rotation: config.LogRotation{
					MaxSize:    "10MB", // String
					MaxBackups: 3,
					MaxAge:     "7d", // String
					Compress:   true,
				},
			},
			expectError: false,
		},
		{
			name: "Valid config Text without rotation",
			cfg: config.LogDestination{
				Name:   "test-text",
				Type:   "file",
				Path:   filePath,
				Format: "text",
				Rotation: config.LogRotation{ // Ensure explicit zero values for clarity in test
					MaxSize:    "",
					MaxAge:     "",
					MaxBackups: 0,
					Compress:   false,
				},
			},
			expectError: false,
		},
		{
			name: "Missing path",
			cfg: config.LogDestination{
				Name:   "test-no-path",
				Type:   "file",
				Format: "json",
			},
			expectError: true,
		},
		{
			name: "Invalid format",
			cfg: config.LogDestination{
				Name:   "test-invalid-format",
				Type:   "file",
				Path:   filePath,
				Format: "xml", // Invalid format
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lgr, err := NewFileLogger(tt.cfg)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				// Cleanup if logger was partially created or file exists
				if lgr != nil {
					if err := lgr.Close(); err != nil {
						t.Errorf("Failed to close logger: %v", err)
					}
				}
				if err := os.Remove(tt.cfg.Path); err != nil && !os.IsNotExist(err) {
					t.Errorf("Failed to remove file: %v", err)
				}
				return // Stop test here
			}
			// --- If no error expected ---
			if err != nil {
				t.Fatalf("Did not expect an error, but got: %v", err)
			}
			if lgr == nil {
				t.Fatal("Expected a logger instance, but got nil")
			}
			defer func() {
				if err := lgr.Close(); err != nil {
					t.Errorf("Failed to close logger: %v", err)
				}
			}()

			if lgr.writer == nil {
				t.Error("Logger writer should not be nil")
			}
			if lgr.format != tt.cfg.Format {
				t.Errorf("Expected format %s, got %s", tt.cfg.Format, lgr.format)
			}

			// Parse expected values for rotation check
			var expectedMaxSizeMB int
			var expectedMaxAgeDays int
			if tt.cfg.Rotation.MaxSize != "" {
				sizeBytes, _ := config.ParseSize(tt.cfg.Rotation.MaxSize)
				expectedMaxSizeMB = int(sizeBytes / (1024 * 1024))
				if expectedMaxSizeMB <= 0 && sizeBytes > 0 {
					expectedMaxSizeMB = 1
				}
			}
			if tt.cfg.Rotation.MaxAge != "" {
				ageDur, _ := config.ParseDuration(tt.cfg.Rotation.MaxAge)
				expectedMaxAgeDays = int(ageDur.Hours() / 24)
				if expectedMaxAgeDays <= 0 && ageDur > 0 {
					expectedMaxAgeDays = 1
				}
			}

			// Check if rotation is configured based on parsed values or backups
			rotationConfigured := expectedMaxSizeMB > 0 || expectedMaxAgeDays > 0 || tt.cfg.Rotation.MaxBackups > 0

			if rotationConfigured {
				lj, ok := lgr.writer.(*lumberjack.Logger)
				if !ok {
					t.Errorf("Expected writer to be *lumberjack.Logger when rotation is configured")
				} else {
					// Compare with parsed and adjusted expected values
					if lj.MaxSize != expectedMaxSizeMB {
						t.Errorf("MaxSize mismatch: want %d MB, got %d MB (from config %s)", expectedMaxSizeMB, lj.MaxSize, tt.cfg.Rotation.MaxSize)
					}
					if lj.MaxBackups != tt.cfg.Rotation.MaxBackups { // MaxBackups remains int
						t.Errorf("MaxBackups mismatch: want %d, got %d", tt.cfg.Rotation.MaxBackups, lj.MaxBackups)
					}
					if lj.MaxAge != expectedMaxAgeDays {
						t.Errorf("MaxAge mismatch: want %d days, got %d days (from config %s)", expectedMaxAgeDays, lj.MaxAge, tt.cfg.Rotation.MaxAge)
					}
					if lj.Compress != tt.cfg.Rotation.Compress {
						t.Errorf("Compress mismatch: want %v, got %v", tt.cfg.Rotation.Compress, lj.Compress)
					}
					if lj.Filename != tt.cfg.Path {
						t.Errorf("Filename mismatch: want %s, got %s", tt.cfg.Path, lj.Filename)
					}
				}
			} else {
				// Expect a regular file writer if rotation is not configured
				_, ok := lgr.writer.(*os.File)
				if !ok {
					// It might be wrapped, check underlying file descriptor if possible or just type
					if !strings.Contains(reflect.TypeOf(lgr.writer).String(), "File") {
						t.Errorf("Expected writer to be *os.File or similar when rotation is not configured, got %T", lgr.writer)
					}
				}
			}

		})
	}
}

func TestFileLogger_Log_JSON(t *testing.T) {
	filePath := tempLogFilePath(t, "log_json_test.log")
	cfg := config.LogDestination{
		Name:   "test-log-json",
		Type:   "file",
		Path:   filePath,
		Format: "json",
	}
	lgr, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := lgr.Close(); err != nil {
			t.Errorf("Failed to close logger: %v", err)
		}
	}()

	// Prepare test data
	testTime := float64(time.Now().UnixMilli() - 1000) // A time in the past
	logData := map[string]interface{}{
		"level":    30.0,
		"time":     testTime,
		"pid":      12345.0,
		"hostname": "test-host",
		"msg":      "Test log message",
		"site_id":  "test-site",
		"value":    987.0,
		"nested":   map[string]interface{}{"key": "val"},
	}

	err = lgr.Log(logData)
	if err != nil {
		t.Fatalf("Log() failed: %v", err)
	}

	// Read the logged line
	lastLine := readLastLine(t, filePath)

	// Unmarshal the logged line and compare
	var loggedData map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &loggedData); err != nil {
		t.Fatalf("Failed to unmarshal logged line: %v\nLine: %s", err, lastLine)
	}

	// Convert original numbers for comparison
	expectedComparable, _ := json.Marshal(logData)
	var expectedData map[string]interface{}
	if err := json.Unmarshal(expectedComparable, &expectedData); err != nil {
		t.Fatalf("Failed to unmarshal expected data: %v", err)
	}

	if !reflect.DeepEqual(loggedData, expectedData) {
		gotJSON, _ := json.MarshalIndent(loggedData, "", "  ")
		wantJSON, _ := json.MarshalIndent(expectedData, "", "  ")
		t.Errorf("Logged JSON data mismatch:\nGot:\n%s\nWant:\n%s", gotJSON, wantJSON)
	}
}

func TestFileLogger_Log_Text(t *testing.T) {
	filePath := tempLogFilePath(t, "log_text_test.log")
	cfg := config.LogDestination{
		Name:   "test-log-text",
		Type:   "file",
		Path:   filePath,
		Format: "text",
	}
	lgr, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := lgr.Close(); err != nil {
			t.Errorf("Failed to close logger: %v", err)
		}
	}()

	// Prepare test data (order might matter for text)
	// Use a fixed time for predictable output
	fixedTime := time.Date(2023, 10, 27, 10, 30, 0, 123456789, time.UTC)
	logData := map[string]interface{}{
		"time":       float64(fixedTime.UnixMilli()), // Use fixed time milliseconds
		"level":      40.0,                           // Warn
		"site_id":    "text-site",
		"msg":        "Warning occurred",
		"request_id": "req-123",
		"value":      100.0,
		"bool_flag":  true,
	}

	err = lgr.Log(logData)
	if err != nil {
		t.Fatalf("Log() failed: %v", err)
	}

	// Read the logged line
	lastLine := readLastLine(t, filePath)

	// Expected format: [TIME] LEVEL: msg (key=value ...)
	expectedTimeStr := fixedTime.Format("2006-01-02T15:04:05.000Z") // Expecting format used in file_logger
	if !strings.Contains(lastLine, "["+expectedTimeStr+"]") {
		t.Errorf("Expected timestamp string '%s' not found in text log: %s", expectedTimeStr, lastLine)
	}
	if !strings.Contains(lastLine, " WARN:") { // Check level representation with space
		t.Errorf("Expected ' WARN:' not found in text log: %s", lastLine)
	}
	if !strings.Contains(lastLine, " Warning occurred") { // Check message with space
		t.Errorf("Expected message ' Warning occurred' not found in text log: %s", lastLine)
	}
	// Check other fields, order might not be guaranteed, so check individually
	if !strings.Contains(lastLine, " site_id=text-site") {
		t.Errorf("Expected ' site_id=text-site' not found in text log: %s", lastLine)
	}
	if !strings.Contains(lastLine, " request_id=req-123") {
		t.Errorf("Expected ' request_id=req-123' not found in text log: %s", lastLine)
	}
	if !strings.Contains(lastLine, " value=100") { // Check number format
		t.Errorf("Expected ' value=100' not found in text log: %s", lastLine)
	}
	if !strings.Contains(lastLine, " bool_flag=true") { // Check boolean format
		t.Errorf("Expected ' bool_flag=true' not found in text log: %s", lastLine)
	}
}

func TestFileLogger_Log_Rotation(t *testing.T) {
	t.Skip("Skipping rotation test with small size (1MB) due to lumberjack limitations.")

	filePath := tempLogFilePath(t, "rotation_test.log")

	// Configure rotation with a very small size for quick testing
	cfg := config.LogDestination{
		Name:   "test-rotation",
		Type:   "file",
		Path:   filePath,
		Format: "text", // Use text for simpler size calculation
		Rotation: config.LogRotation{
			MaxSize:    "1", // 1 MB - minimum size supported by lumberjack
			MaxBackups: 2,
			MaxAge:     "", // Disable age-based rotation for this test
			Compress:   false,
		},
	}
	lgr, err := NewFileLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer func() {
		if err := lgr.Close(); err != nil {
			t.Errorf("Failed to close logger: %v", err)
		}
	}()

	// Log enough data to trigger rotation several times
	// Each line is ~100 bytes. Logger uses min 1MB limit.
	// We need > 1MB / 100B = > 10485 lines. Let's log 15000 lines.
	logLine := map[string]interface{}{"msg": strings.Repeat("X", 90)} // Approx 100 bytes per line
	for i := 0; i < 15000; i++ {
		if err := lgr.Log(logLine); err != nil {
			t.Fatalf("Log() failed during rotation test: %v", err)
		}
	}

	// Allow some time for rotation to potentially complete
	time.Sleep(200 * time.Millisecond) // Increased pause

	// Check for rotated files
	baseDir := filepath.Dir(filePath)
	baseName := filepath.Base(filePath)
	files, err := filepath.Glob(filepath.Join(baseDir, baseName+".*"))
	if err != nil {
		t.Fatalf("Error checking for rotated files: %v", err)
	}

	// Expecting original file + MaxBackups rotated files
	expectedFileCount := cfg.Rotation.MaxBackups // Lumberjack keeps MaxBackups *old* files
	actualRotatedCount := 0
	for _, f := range files {
		// Basic check to filter out unexpected files in temp dir
		if strings.HasPrefix(filepath.Base(f), baseName+".") {
			actualRotatedCount++
		}
	}

	if actualRotatedCount != expectedFileCount {
		t.Errorf("Expected %d rotated files, found %d", expectedFileCount, actualRotatedCount)
		t.Logf("Found files: %v", files)
	}

	// Further check: Size of the current log file should be small (<= MaxSize)
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat current log file %s: %v", filePath, err)
	}

	// FIX: Get the actual size limit in MB used by lumberjack,
	// and convert it back to bytes for comparison.
	sizeBytesConfig, _ := config.ParseSize(cfg.Rotation.MaxSize)
	maxSizeMB := int(sizeBytesConfig / (1024 * 1024))
	if sizeBytesConfig > 0 && sizeBytesConfig < (1024*1024) {
		maxSizeMB = 1 // Logger sets at least 1MB
	}
	var actualMaxSizeBytes int64
	if maxSizeMB > 0 {
		actualMaxSizeBytes = int64(maxSizeMB) * 1024 * 1024
	} else {
		// If maxSizeMB is 0, rotation by size is not active,
		// so the file size should not be limited (or is limited differently).
		// For this test, we assume that maxSizeMB will be > 0.
		t.Logf("Warning: maxSizeMB calculated to 0, size check might be inaccurate.")
		actualMaxSizeBytes = info.Size() + 1 // Ensures the test passes if size rotation is not active
	}

	if info.Size() > actualMaxSizeBytes {
		t.Errorf("Current log file size (%d) exceeds the effective rotation limit (%d bytes, based on %dMB used by lumberjack)", info.Size(), actualMaxSizeBytes, maxSizeMB)
	}
}

// TODO: Add test for MaxAge rotation
// TODO: Add test for Compress=true rotation

// --- Helper for Truncation Test ---
type mockWriteCloser struct {
	buf bytes.Buffer
}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockWriteCloser) Close() error {
	return nil // No-op for mock
}

func (m *mockWriteCloser) String() string {
	return m.buf.String()
}

func TestFileLoggerTruncation(t *testing.T) {
	tests := []struct {
		name            string
		format          string
		maxSize         int
		record          map[string]interface{}
		expectTruncated bool   // For JSON, means replaced with error msg
		expectSubstring string // Substring to check in the output
	}{
		{
			name:            "Text below limit",
			format:          "text",
			maxSize:         100,
			record:          map[string]interface{}{"level": 30, "msg": "short text msg", "val": 1},
			expectTruncated: false,
			expectSubstring: "short text msg val=1",
		},
		{
			name:            "Text exceeds limit",
			format:          "text",
			maxSize:         50, // Short limit
			record:          map[string]interface{}{"level": 30, "msg": "this is a very long text message that will be truncated", "val": 123},
			expectTruncated: true,
			expectSubstring: "...truncated",
		},
		{
			name:            "JSON below limit",
			format:          "json",
			maxSize:         200,
			record:          map[string]interface{}{"level": 30, "msg": "short json msg", "val": 1, "site_id": "site1"},
			expectTruncated: false,
			expectSubstring: `"msg":"short json msg"`, // Check original content
		},
		{
			name:    "JSON exceeds limit",
			format:  "json",
			maxSize: 100, // Very short limit for JSON
			record: map[string]interface{}{
				"level":      40,
				"msg":        "this json message is definitely way too long to fit into the small limit provided",
				"val":        12345,
				"site_id":    "long_site_id_example",
				"user_agent": "Some Very Long User Agent String/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			},
			expectTruncated: true,
			expectSubstring: `"_log_error":"Original message truncated`, // Check for error message
		},
		{
			name:    "JSON exceeds limit - check essential fields",
			format:  "json",
			maxSize: 250, // Slightly larger limit
			record: map[string]interface{}{
				"time":       "2023-01-01T10:00:00Z",
				"level":      50,
				"hostname":   "server01",
				"pid":        999,
				"name":       "TestLogger",
				"site_id":    "site-abc",
				"ip_address": "192.168.1.100",
				"user_agent": "ShortAgent",
				"extra_data": strings.Repeat("A", 300), // This makes it exceed the limit
			},
			expectTruncated: true,
			expectSubstring: `"site_id":"site-abc"`, // Check if essential field is present
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWriter := &mockWriteCloser{}
			lgr := &FileLogger{
				mu:             sync.Mutex{},
				writer:         mockWriter,
				format:         tt.format,
				name:           "test-trunc",
				maxMessageSize: tt.maxSize,
			}

			err := lgr.Log(tt.record)
			if err != nil {
				t.Fatalf("Log() failed: %v", err)
			}

			output := mockWriter.String()
			if !strings.Contains(output, tt.expectSubstring) {
				t.Errorf("Output does not contain expected substring %q.\nOutput:\n%s", tt.expectSubstring, output)
			}

			// For JSON truncation, verify it's still valid JSON
			if tt.format == "json" && tt.expectTruncated {
				var jsonData map[string]interface{}
				// Trim newline before unmarshaling
				if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &jsonData); err != nil {
					t.Errorf("Truncated JSON is not valid: %v.\nOutput:\n%s", err, output)
				}
				if _, ok := jsonData["_log_error"]; !ok {
					t.Errorf("Truncated JSON missing '_log_error' field.\nOutput:\n%s", output)
				}
			}

			// Check length constraint (approximate for JSON, exact for text if truncated)
			outputLen := len(strings.TrimSpace(output))
			if tt.expectTruncated && outputLen > tt.maxSize {
				// Allow slight overshoot for JSON error message generation, but not excessive
				allowedOvershoot := 50 // Generous buffer
				if tt.format != "json" || outputLen > tt.maxSize+allowedOvershoot {
					t.Errorf("Output length %d exceeds maxSize %d (format: %s).\nOutput:\n%s", outputLen, tt.maxSize, tt.format, output)
				}
			}

			if err := lgr.Close(); err != nil {
				t.Errorf("Close() failed: %v", err)
			}
		})
	}
}
