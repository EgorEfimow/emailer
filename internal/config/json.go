package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// loadJSON overrides cfg fields from a JSON file at path.
// Fields not present in the file are left unchanged (defaults are preserved).
// Returns an error if the file cannot be read or the JSON is malformed.
//
// Internally uses yaml.v3 because it natively unmarshals duration strings
// into time.Duration (encoding/json does not), and JSON is valid YAML.
func loadJSON(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config.loadJSON: %w", err)
	}

	// Check for empty or whitespace-only file before unmarshalling.
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	// Unmarshal into a temporary map to detect empty objects.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config.loadJSON: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}

	// Unmarshal into the existing config. yaml.Unmarshal into a non-nil
	// struct pointer only overwrites fields that are present in the JSON
	// document, preserving any pre-set values (e.g. defaults) for omitted
	// fields.
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("config.loadJSON: %w", err)
	}

	return nil
}