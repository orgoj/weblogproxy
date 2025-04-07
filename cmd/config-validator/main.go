package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/orgoj/weblogproxy/internal/config"
)

func main() {
	// Parse command line flags
	flag.Parse()

	// Get config path from arguments
	if len(flag.Args()) < 1 {
		fmt.Println("Error: Config file path is required")
		fmt.Println("Usage: config-validator <config-file>")
		os.Exit(1)
	}
	configPath := flag.Args()[0]

	// Load and validate configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Perform additional validation
	if err := validateConfig(cfg); err != nil {
		fmt.Printf("Validation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration is valid!")
}

func validateConfig(cfg *config.Config) error {
	// Check server configuration
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}

	if cfg.Server.Mode != "standalone" && cfg.Server.Mode != "embedded" {
		return fmt.Errorf("invalid server mode: %s (must be 'standalone' or 'embedded')", cfg.Server.Mode)
	}

	// Check security configuration
	if cfg.Security.Token.Secret == "" {
		return fmt.Errorf("security token secret cannot be empty")
	}

	// Use ParseDuration for token expiration validation
	_, err := config.ParseDuration(cfg.Security.Token.Expiration)
	if err != nil {
		return fmt.Errorf("invalid security token expiration: %w", err)
	}

	// Check log destinations
	hasEnabledDestination := false
	for i, dest := range cfg.LogDestinations {
		if !dest.Enabled {
			continue
		}

		hasEnabledDestination = true

		switch dest.Type {
		case "gelf":
			if dest.Host == "" {
				return fmt.Errorf("log destination %d: GELF host cannot be empty", i)
			}
			if dest.Port <= 0 || dest.Port > 65535 {
				return fmt.Errorf("log destination %d: invalid GELF port: %d", i, dest.Port)
			}
		case "file":
			if dest.Path == "" {
				return fmt.Errorf("log destination %d: file path cannot be empty", i)
			}
		default:
			return fmt.Errorf("log destination %d: unsupported type: %s", i, dest.Type)
		}
	}

	if !hasEnabledDestination {
		return fmt.Errorf("at least one log destination must be enabled")
	}

	return nil
}
