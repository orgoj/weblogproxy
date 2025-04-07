// internal/logger/interface.go

package logger

// Logger defines the interface for all log destination implementations.
// Each logger type (file, gelf, etc.) will implement this interface.
type Logger interface {
	// Log processes and sends a single log record.
	// The record is provided as a map, representing the fully merged and enriched
	// internal representation before destination-specific formatting.
	// Implementations are responsible for transforming this map into the
	// required output format (e.g., Bunyan JSON, GELF JSON).
	Log(record map[string]interface{}) error

	// Close handles any necessary cleanup, like flushing buffers or closing connections.
	// It should be called during application shutdown.
	Close() error

	// Name returns the unique name of the logger instance (from config).
	Name() string
}
