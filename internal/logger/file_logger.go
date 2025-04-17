// internal/logger/file_logger.go

package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/orgoj/weblogproxy/internal/config" // Assuming config path
	"gopkg.in/natefinch/lumberjack.v2"
)

// FileLogger handles logging to a file with optional rotation.
type FileLogger struct {
	mu             sync.Mutex
	writer         io.WriteCloser // Can be *os.File or *lumberjack.Logger
	format         string         // "json" or "text"
	name           string         // Added to store logger name
	maxMessageSize int            // Max size in bytes, 0 means unlimited
	appLogger      *AppLogger     // For internal logging
}

// NewFileLogger creates a new FileLogger instance.
func NewFileLogger(cfg config.LogDestination) (*FileLogger, error) {
	appLogger := GetAppLogger()

	if cfg.Path == "" {
		return nil, fmt.Errorf("file logger requires a path")
	}
	if cfg.Format != "json" && cfg.Format != "text" {
		return nil, fmt.Errorf("invalid file logger format: %s", cfg.Format)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("file logger requires a name") // Added check for name
	}

	var writer io.WriteCloser
	var maxSizeMB int
	var maxAgeDays int

	// Parse rotation config if provided
	if cfg.Rotation.MaxSize != "" {
		sizeBytes, err := config.ParseSize(cfg.Rotation.MaxSize)
		if err != nil {
			return nil, fmt.Errorf("invalid rotation.max_size: %w", err)
		}
		// Convert bytes to MB, ensuring at least 1MB (lumberjack requires positive values)
		maxSizeMB = int(sizeBytes / 1024 / 1024)
		if maxSizeMB < 1 && sizeBytes > 0 {
			maxSizeMB = 1 // Minimum 1MB for any positive size
		}
	}

	if cfg.Rotation.MaxAge != "" {
		duration, err := config.ParseDuration(cfg.Rotation.MaxAge)
		if err != nil {
			return nil, fmt.Errorf("invalid rotation.max_age: %w", err)
		}
		// Convert duration to days, flooring to integer
		maxAgeDays = int(duration.Hours() / 24)
	}

	rotationConfigured := maxSizeMB > 0 || maxAgeDays > 0 || cfg.Rotation.MaxBackups > 0

	if rotationConfigured {
		appLogger.Info("Configuring file rotation for '%s': MaxSize=%dMB, MaxAge=%ddays, MaxBackups=%d, Compress=%t",
			cfg.Path, maxSizeMB, maxAgeDays, cfg.Rotation.MaxBackups, cfg.Rotation.Compress)
		writer = &lumberjack.Logger{
			Filename:   cfg.Path,
			MaxSize:    maxSizeMB, // Use the parsed value in MB
			MaxBackups: cfg.Rotation.MaxBackups,
			MaxAge:     maxAgeDays, // Use the parsed value in days
			Compress:   cfg.Rotation.Compress,
			LocalTime:  false, // Use UTC time for logs
		}
	} else {
		// Otherwise, use a standard file
		file, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640) // #nosec G302 -- Permissions set to 0640 to allow reading by log collectors in the same group.
		if err != nil {
			appLogger.Error("Failed to open log file %s: %v", cfg.Path, err)
			return nil, fmt.Errorf("failed to open log file %s: %w", cfg.Path, err)
		}
		writer = file
	}

	// Determine max message size
	maxSize := cfg.MaxMessageSize
	if maxSize <= 0 { // Default for file is 4096 if not set or invalid
		maxSize = 4096
	}

	return &FileLogger{
		writer:         writer,
		format:         cfg.Format,
		name:           cfg.Name, // Store the name
		maxMessageSize: maxSize,
		appLogger:      appLogger,
	}, nil
}

// Log writes the log record to the file.
func (l *FileLogger) Log(record map[string]interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var line []byte
	var err error

	if l.format == "json" {
		// Ensure standard fields for Bunyan compatibility if missing?
		// Or assume Enricher already added them?
		// For now, assume they exist if needed by the format.

		line, err = json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal log record to JSON: %w", err)
		}
		// Check size *before* appending newline for JSON
		if l.maxMessageSize > 0 && len(line) > l.maxMessageSize {
			l.appLogger.Warn("Log message for destination '%s' truncated (JSON format). Size: %d > Limit: %d", l.name, len(line), l.maxMessageSize)
			line = createTruncatedJSONRecord(record, l.maxMessageSize)
		}
		line = append(line, '\n') // Append newline for JSON Lines format
	} else { // format == "text"
		line = l.formatText(record)
		// Check size *before* appending newline for text
		if l.maxMessageSize > 0 && len(line) > l.maxMessageSize {
			l.appLogger.Warn("Log message for destination '%s' truncated (Text format). Size: %d > Limit: %d", l.name, len(line), l.maxMessageSize)
			line = []byte(truncateString(string(line), l.maxMessageSize))
		}
		line = append(line, '\n')
	}

	_, err = l.writer.Write(line)
	if err != nil {
		return fmt.Errorf("failed to write log line: %w", err)
	}

	return nil
}

// formatText converts the record map into a simple text line format.
// Example: [TIME] LEVEL: msg (key=value key2=value2 ...)
func (l *FileLogger) formatText(record map[string]interface{}) []byte {
	var sb strings.Builder

	// Timestamp
	timestamp := time.Now().UTC() // Default to now if not present
	if timeVal, ok := record["time"]; ok {
		if tsStr, ok := timeVal.(string); ok {
			// Parse ISO 8601 timestamp string
			if parsed, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
				timestamp = parsed
			}
		} else if tsFloat, ok := timeVal.(float64); ok {
			// Backward compatibility for older numeric timestamps
			sec := int64(tsFloat / 1000)
			nsec := int64(tsFloat) % 1000 * 1000000
			timestamp = time.Unix(sec, nsec).UTC()
		}
	}
	sb.WriteString("[")
	sb.WriteString(timestamp.Format("2006-01-02T15:04:05.000Z")) // Consistent timestamp format
	sb.WriteString("]")

	// Level
	levelStr := "INFO" // Default
	if levelVal, ok := record["level"]; ok {
		if levelFloat, ok := levelVal.(float64); ok {
			levelStr = levelToString(int(levelFloat))
		} else if levelInt, ok := levelVal.(int); ok {
			levelStr = levelToString(levelInt)
		}
	}
	sb.WriteString(" ")
	sb.WriteString(levelStr)
	sb.WriteString(":")

	// Message
	msg := "-" // Default
	if msgVal, ok := record["msg"]; ok {
		if msgStr, ok := msgVal.(string); ok {
			msg = msgStr
		}
	}
	sb.WriteString(" ")
	sb.WriteString(msg)

	// Other fields (sorted for consistency)
	keys := make([]string, 0, len(record))
	for k := range record {
		// Skip already processed fields
		if k == "time" || k == "level" || k == "msg" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(formatValue(record[k]))
	}

	return []byte(sb.String())
}

// formatValue converts different types to string for text logging.
func formatValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Quote strings containing spaces?
		if strings.Contains(v, " ") {
			return strconv.Quote(v)
		}
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return "<nil>"
	default:
		// For complex types (maps, slices), marshal to JSON?
		jsonBytes, err := json.Marshal(v)
		if err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%v", v) // Fallback to default Go formatting
	}
}

// levelToString converts Bunyan-like level numbers to strings.
func levelToString(level int) string {
	switch {
	case level <= 10: // TRACE
		return "TRACE"
	case level <= 20: // DEBUG
		return "DEBUG"
	case level <= 30: // INFO
		return "INFO"
	case level <= 40: // WARN
		return "WARN"
	case level <= 50: // ERROR
		return "ERROR"
	default: // FATAL and above
		return "FATAL"
	}
}

// Close closes the underlying file writer.
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.writer != nil {
		return l.writer.Close()
	}
	return nil
}

// Name returns the name of the logger destination.
func (l *FileLogger) Name() string {
	return l.name
}

// Ensure FileLogger implements the Logger interface.
var _ Logger = (*FileLogger)(nil)

// createTruncatedJSONRecord creates a minimal JSON record indicating truncation.
func createTruncatedJSONRecord(originalRecord map[string]interface{}, maxSize int) []byte {
	truncatedRecord := make(map[string]interface{})

	// Add error message
	truncatedRecord["_log_error"] = "Original message truncated due to size limit"

	// Copy essential fields, potentially truncating long strings
	essentialFields := []string{"time", "level", "hostname", "pid", "name", "site_id", "ip_address", "user_agent", "x_forwarded_for"}
	// Estimate overhead for the error message and basic JSON structure
	overheadEstimate := 200
	// Give some buffer for each field key/quotes/commas
	perFieldBuffer := 30
	availableStringSpace := maxSize - overheadEstimate - (len(essentialFields) * perFieldBuffer)
	if availableStringSpace < 50 { // Ensure some minimum space
		availableStringSpace = 50
	}
	// Allocate space per field roughly
	maxLengthPerField := availableStringSpace / len(essentialFields)
	if maxLengthPerField < 10 {
		maxLengthPerField = 10
	}

	for _, key := range essentialFields {
		if val, ok := originalRecord[key]; ok {
			if strVal, ok := val.(string); ok {
				truncatedRecord[key] = truncateString(strVal, maxLengthPerField)
			} else {
				// Keep non-string essential fields as they are (usually small)
				truncatedRecord[key] = val
			}
		}
	}

	// Marshal the truncated record
	line, err := json.Marshal(truncatedRecord)
	if err != nil {
		// Fallback to a very basic error message if marshaling fails
		failureMsg := fmt.Sprintf(`{"_log_error":"Original message truncated and failed to create detailed truncation record: %v"}`, err)
		// Ensure even the fallback fits (highly unlikely to fail here)
		if len(failureMsg) > maxSize {
			return []byte(failureMsg[:maxSize])
		}
		return []byte(failureMsg)
	}

	// Final check if the generated truncated message is still too big (e.g., due to many fields)
	if len(line) > maxSize {
		// Just return the error part, truncated if needed
		errPart := `{"_log_error":"Original message truncated due to size limit (details also truncated)"}`
		if len(errPart) > maxSize {
			return []byte(errPart[:maxSize])
		}
		return []byte(errPart)
	}

	return line
}

// truncateString removed - moved to utils.go
