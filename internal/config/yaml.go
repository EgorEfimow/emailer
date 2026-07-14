package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// loadYAML overrides cfg fields from a YAML file at path.
// Fields not present in the file are left unchanged (defaults are preserved).
// Returns an error if the file cannot be read or the YAML is malformed.
func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config.loadYAML: %w", err)
	}

	// Unmarshal into a temporary map to detect empty files before applying.
	// An empty file (or a file with only whitespace/comments) produces no
	// keys, so we short-circuit without error.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config.loadYAML: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}

	// Unmarshal into the existing config. yaml.Unmarshal into a non-nil
	// struct pointer only overwrites fields that are present in the YAML
	// document, preserving any pre-set values (e.g. defaults) for omitted
	// fields.
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("config.loadYAML: %w", err)
	}

	return nil
}