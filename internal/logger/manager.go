// internal/logger/manager.go

package logger

import (
	"fmt"
	"sync"

	"github.com/orgoj/weblogproxy/internal/config"
)

// Manager handles the lifecycle and access to logger instances.
type Manager struct {
	loggers   map[string]Logger
	mu        sync.RWMutex
	appLogger *AppLogger
}

// NewManager creates a new logger manager.
func NewManager() *Manager {
	return &Manager{
		loggers:   make(map[string]Logger),
		appLogger: GetAppLogger(),
	}
}

// InitLoggers initializes loggers based on the provided configuration.
func (m *Manager) InitLoggers(destinations []config.LogDestination) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing loggers first if any (e.g., on config reload)
	for name, lgr := range m.loggers {
		if err := lgr.Close(); err != nil {
			m.appLogger.Warn("Error closing existing logger '%s' during re-initialization: %v", name, err)
		}
	}
	m.loggers = make(map[string]Logger) // Reset the map

	var initErrors []error
	for _, dest := range destinations {
		if !dest.Enabled {
			continue
		}

		var lgr Logger
		var err error

		switch dest.Type {
		case "file":
			lgr, err = NewFileLogger(dest)
		case "gelf":
			lgr, err = NewGelfLogger(dest)
		// case "stdout":
		// 	lgr, err = NewStdoutLogger(dest)
		default:
			err = fmt.Errorf("unsupported logger type: %s", dest.Type)
		}

		if err != nil {
			m.appLogger.Error("Failed to initialize logger destination '%s' (type: %s): %v", dest.Name, dest.Type, err)
			initErrors = append(initErrors, fmt.Errorf("dest '%s': %w", dest.Name, err))
			continue
		}

		m.loggers[dest.Name] = lgr
		m.appLogger.Info("Initialized logger destination '%s' (type: %s)", dest.Name, dest.Type)
	}

	if len(initErrors) > 0 {
		// Combine errors? For now, just return the first one or a generic error.
		return fmt.Errorf("failed to initialize some loggers: %v", initErrors)
	}
	return nil
}

// GetLogger retrieves a logger instance by name.
// Returns nil if the logger is not found or not initialized.
func (m *Manager) GetLogger(name string) Logger {
	m.mu.RLock()
	defer m.mu.RUnlock()
	lgr, ok := m.loggers[name]
	if !ok {
		return nil
	}
	return lgr
}

// GetAllEnabledLoggerNames returns a slice of names for all initialized loggers.
func (m *Manager) GetAllEnabledLoggerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.loggers))
	for name := range m.loggers {
		names = append(names, name)
	}
	return names
}

// CloseAll closes all managed logger instances.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.appLogger.Info("Shutting down... Closing loggers.")
	var wg sync.WaitGroup
	for name, lgr := range m.loggers {
		wg.Add(1)
		go func(name string, lgr Logger) {
			defer wg.Done()
			if err := lgr.Close(); err != nil {
				m.appLogger.Warn("Error closing logger '%s': %v", name, err)
			}
		}(name, lgr)
	}
	wg.Wait()
	m.appLogger.Info("Loggers closed.")
	m.loggers = make(map[string]Logger) // Clear the map after closing
}
