// internal/logger/gelf_logger.go

package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/orgoj/weblogproxy/internal/config"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

// Variables for factories to allow mocking in tests
var gelfUDPWriterFactory = gelf.NewUDPWriter
var gelfTCPWriterFactory = gelf.NewTCPWriter

// Function to set compression, can be mocked in tests
var setUDPCompression = func(writer *gelf.UDPWriter, compType gelf.CompressType) {
	writer.CompressionType = compType
}

// GelfLogger implements the Logger interface for GELF logs
type GelfLogger struct {
	name       string
	writer     gelf.Writer
	hostName   string
	addLogData []config.AddLogDataSpec
}

// NewGelfLogger creates a new GELF logger
func NewGelfLogger(cfg config.LogDestination) (*GelfLogger, error) {
	// Validate config
	if cfg.Host == "" {
		return nil, fmt.Errorf("host is required for GELF logger")
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("valid port is required for GELF logger")
	}

	var writer gelf.Writer
	var err error

	hostName, err := os.Hostname()
	if err != nil {
		hostName = "unknown"
		fmt.Printf("[WARN] Failed to get hostname: %v, using '%s'\n", err, hostName)
	}

	// Construct address
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	// Create the appropriate writer based on protocol
	if cfg.Protocol == "tcp" {
		tcpWriter, err := gelfTCPWriterFactory(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create GELF TCP writer: %w", err)
		}
		writer = tcpWriter
	} else {
		// Default to UDP
		udpWriter, err := gelfUDPWriterFactory(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create GELF UDP writer: %w", err)
		}

		// Set compression based on config
		switch cfg.CompressionType {
		case "gzip":
			setUDPCompression(udpWriter, gelf.CompressGzip)
		case "zlib":
			setUDPCompression(udpWriter, gelf.CompressZlib)
		default:
			// Default to none
			setUDPCompression(udpWriter, gelf.CompressNone)
		}

		writer = udpWriter
	}

	return &GelfLogger{
		name:       cfg.Name,
		writer:     writer,
		hostName:   hostName,
		addLogData: cfg.AddLogData,
	}, nil
}

// Log sends a record to the Graylog server
func (g *GelfLogger) Log(record map[string]interface{}) error {
	// Create a GELF message
	msg := &gelf.Message{
		Version:  "1.1",
		Host:     g.hostName,
		Short:    getString(record, "message", "No message"),
		TimeUnix: getTimestamp(record),
		Level:    getLevel(record),
		Extra:    make(map[string]interface{}),
	}

	// Full message is optional
	if fullMsg, ok := record["full_message"]; ok {
		if fullMsgStr, ok := fullMsg.(string); ok {
			msg.Full = fullMsgStr
		}
	}

	// Add all other fields as extra fields
	for k, v := range record {
		// Skip fields that are already set as standard GELF fields
		if k == "message" || k == "timestamp" || k == "level" || k == "full_message" {
			continue
		}

		// GELF requires additional fields to start with an underscore
		extraKey := k
		if extraKey[0] != '_' {
			extraKey = "_" + extraKey
		}

		// GELF doesn't support complex data types
		switch v := v.(type) {
		case string, float64, float32, int, int32, int64, uint, uint32, uint64:
			msg.Extra[extraKey] = v
		default:
			// Convert to string for other types
			msg.Extra[extraKey] = fmt.Sprintf("%v", v)
		}
	}

	// Send the message
	return g.writer.WriteMessage(msg)
}

// Close closes the GELF writer
func (g *GelfLogger) Close() error {
	return g.writer.Close()
}

// Name returns the name of the logger
func (g *GelfLogger) Name() string {
	return g.name
}

// Helper function to get string value from record
func getString(record map[string]interface{}, key, defaultValue string) string {
	if val, ok := record[key]; ok {
		if strVal, ok := val.(string); ok {
			return strVal
		}
		return fmt.Sprintf("%v", val)
	}
	return defaultValue
}

// Helper function to get timestamp from record or use current time
func getTimestamp(record map[string]interface{}) float64 {
	if ts, ok := record["timestamp"]; ok {
		switch v := ts.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		case time.Time:
			return float64(v.Unix()) + float64(v.Nanosecond())/1e9
		}
	}
	return float64(time.Now().Unix()) + float64(time.Now().Nanosecond())/1e9
}

// Helper function to get log level from record
func getLevel(record map[string]interface{}) int32 {
	if lvl, ok := record["level"]; ok {
		switch v := lvl.(type) {
		case int:
			// Bezpečná konverze z int na int32 s ověřením rozsahu
			if v < 0 {
				return 0
			} else if v > 7 {
				return 7
			}
			return int32(v)
		case int32:
			return v
		case int64:
			// Bezpečná konverze z int64 na int32 s ověřením rozsahu
			if v < 0 {
				return 0
			} else if v > 7 {
				return 7
			}
			return int32(v)
		case float64:
			// Bezpečná konverze z float64 na int32 s ověřením rozsahu
			if v < 0 {
				return 0
			} else if v > 7 {
				return 7
			}
			return int32(v)
		case string:
			// Convert string levels to numeric
			switch v {
			case "emergency", "emerg":
				return 0
			case "alert":
				return 1
			case "critical", "crit":
				return 2
			case "error", "err":
				return 3
			case "warning", "warn":
				return 4
			case "notice":
				return 5
			case "informational", "info":
				return 6
			case "debug":
				return 7
			}
		}
	}
	return 6 // Default to INFO level
}
