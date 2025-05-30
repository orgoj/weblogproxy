package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/orgoj/weblogproxy/internal/server"
	"github.com/orgoj/weblogproxy/internal/version"
)

func main() {
	// --- Configuration --- //
	configPath := flag.String("config", "config/config.yaml", "Path to the configuration file")
	testConfigShort := flag.Bool("t", false, "Test configuration and exit (nginx style)")
	testConfigLong := flag.Bool("test", false, "Test configuration and exit (nginx style)")
	showVersion := flag.Bool("version", false, "Show version information and exit")
	flag.Parse()

	// Display version information if requested
	if *showVersion {
		fmt.Println(version.VersionInfo())
		os.Exit(0)
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("[CRITICAL] Failed to load configuration from '%s': %v\n", *configPath, err)
		os.Exit(1)
	}

	// Validate the loaded configuration
	if err := config.ValidateConfig(cfg); err != nil {
		fmt.Printf("[CRITICAL] Configuration validation failed for '%s':\n%v\n", *configPath, err)
		os.Exit(1)
	}

	if *testConfigShort || *testConfigLong {
		// Validation was already done above
		fmt.Printf("Configuration '%s' is valid.\n", *configPath)
		os.Exit(0)
	}

	// Initialize application logger
	appLogger := logger.GetAppLogger()
	if err := appLogger.SetLogLevelFromString(cfg.AppLog.Level); err != nil {
		fmt.Printf("[WARN] Invalid log level '%s', using default: %v\n", cfg.AppLog.Level, err)
	}
	appLogger.SetShowHealth(cfg.AppLog.ShowHealthLogs)

	// Log the version at startup
	appLogger.Warn("%s", version.VersionInfo())

	// --- Dependency Initialization --- //

	// Initialize Logger Manager
	loggerManager := logger.NewManager()
	if err := loggerManager.InitLoggers(cfg.LogDestinations); err != nil {
		appLogger.Fatal("Failed to initialize one or more loggers: %v. Exiting.", err)
	}
	defer loggerManager.CloseAll()

	// Initialize Rule Processor
	ruleProcessor, err := rules.NewRuleProcessor(cfg)
	if err != nil {
		appLogger.Fatal("Failed to initialize rule processor: %v", err)
	}

	// Prepare server dependencies
	serverDeps := server.Dependencies{
		Config:        cfg,
		LoggerManager: loggerManager,
		RuleProcessor: ruleProcessor,
		AppLogger:     appLogger,
	}

	// --- Server Setup --- //

	// Create Server instance
	srv := server.NewServer(serverDeps)

	// --- Config Reload Goroutine --- //
	if cfg.ConfigReload.Enabled && cfg.ConfigReload.Interval > 0 {
		configPathCopy := *configPath
		interval := time.Duration(cfg.ConfigReload.Interval) * time.Second
		lastModTime := func() time.Time {
			info, err := os.Stat(configPathCopy)
			if err != nil {
				return time.Time{}
			}
			return info.ModTime()
		}()

		go func() {
			for {
				time.Sleep(interval)
				info, err := os.Stat(configPathCopy)
				if err != nil {
					fmt.Fprintf(os.Stdout, "[ERROR] Config reload: cannot stat config file: %v\n", err)
					continue
				}
				if info.ModTime().After(lastModTime) {
					fmt.Fprintf(os.Stdout, "[INFO] Config reload: detected change, reloading...\n")
					newCfg, err := config.LoadConfig(configPathCopy)
					if err != nil {
						fmt.Fprintf(os.Stdout, "[ERROR] Config reload: failed to load: %v\n", err)
						continue
					}
					if err := config.ValidateConfig(newCfg); err != nil {
						fmt.Fprintf(os.Stdout, "[ERROR] Config reload: validation failed: %v\n", err)
						continue
					}
					// Re-init loggerManager and ruleProcessor
					if err := loggerManager.InitLoggers(newCfg.LogDestinations); err != nil {
						fmt.Fprintf(os.Stdout, "[ERROR] Config reload: failed to re-init loggers: %v\n", err)
						continue
					}
					newRuleProcessor, err := rules.NewRuleProcessor(newCfg)
					if err != nil {
						fmt.Fprintf(os.Stdout, "[ERROR] Config reload: failed to re-init rule processor: %v\n", err)
						continue
					}
					// Safe update runtime config
					cfg = newCfg
					ruleProcessor = newRuleProcessor
					fmt.Fprintf(os.Stdout, "[INFO] Config reload: applied new configuration.\n")
					lastModTime = info.ModTime()
				}
			}
		}()
	}

	// --- Graceful Shutdown --- //

	// Start server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	appLogger.Info("Received shutdown signal.")

	// The context is used to inform the server it has 5 seconds to finish
	// the requests it is currently handling
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	// TODO: Implement graceful server shutdown using http.Server
	// if err := srv.Shutdown(ctx); err != nil {
	// 	 fmt.Printf("Server forced to shutdown: %v\n", err)
	// }

	// Close loggers (already deferred)
	// loggerManager.CloseAll()

	appLogger.Info("WebLogProxy shut down gracefully.")
	os.Exit(0)
}
