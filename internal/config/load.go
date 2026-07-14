package config

import (
	"fmt"
	"os"
	"strings"
)

// LoadOptions specifies optional configuration sources for Load.
type LoadOptions struct {
	// ConfigPath is the path to a YAML (.yaml/.yml) or JSON (.json)
	// configuration file. When empty, no file is loaded.
	ConfigPath string

	// Args holds CLI arguments for flag parsing. When nil, os.Args[1:]
	// is used.
	Args []string
}

// Load builds a Config by layering sources in precedence order
// (later overrides earlier):
//
//	1. Defaults (from DefaultConfig)
//	2. YAML file (if ConfigPath ends in .yaml or .yml)
//	3. JSON file (if ConfigPath ends in .json)
//	4. Environment variables
//	5. CLI flags
//
// Returns an error if any source fails to load.
func Load(opts LoadOptions) (Config, error) {
	cfg := DefaultConfig()

	// 1. Config file (YAML or JSON)
	if opts.ConfigPath != "" {
		if err := loadFile(opts.ConfigPath, &cfg); err != nil {
			return Config{}, err
		}
	}

	// 2. Environment variables
	if err := loadEnv(&cfg); err != nil {
		return Config{}, err
	}

	// 3. CLI flags
	args := opts.Args
	if args == nil {
		args = os.Args[1:]
	}
	if err := loadFlags(args, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// loadFile detects the config format by file extension and loads it.
// Supported extensions: .yaml, .yml, .json.
func loadFile(path string, cfg *Config) error {
	switch {
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		if err := loadYAML(path, cfg); err != nil {
			return fmt.Errorf("config.Load: %w", err)
		}
		return nil
	case strings.HasSuffix(path, ".json"):
		if err := loadJSON(path, cfg); err != nil {
			return fmt.Errorf("config.Load: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("config.Load: unsupported config file extension: %q (supported: .yaml, .yml, .json)", path)
	}
}