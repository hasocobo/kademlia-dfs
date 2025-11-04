package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the application configuration
type Config struct {
	// Node configuration
	NodeID      string `json:"node_id,omitempty"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	StoragePath string `json:"storage_path"`

	// Bootstrap nodes
	BootstrapNodes []string `json:"bootstrap_nodes,omitempty"`

	// Kademlia parameters
	K     int `json:"k"`     // Bucket size
	Alpha int `json:"alpha"` // Concurrency parameter
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		IP:          "127.0.0.1",
		Port:        4001,
		StoragePath: filepath.Join(os.Getenv("HOME"), ".kademlia-dfs", "storage"),
		K:           20,
		Alpha:       3,
	}
}

// Load loads configuration from file or returns default
func Load() (*Config, error) {
	configPath := getConfigPath()

	// Try to load from file
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			cfg := DefaultConfig()
			// Try to save default config
			if err := Save(cfg); err != nil {
				// Ignore save errors
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Use defaults for missing fields
	if cfg.K == 0 {
		cfg.K = 20
	}
	if cfg.Alpha == 0 {
		cfg.Alpha = 3
	}
	if cfg.IP == "" {
		cfg.IP = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 4001
	}
	if cfg.StoragePath == "" {
		cfg.StoragePath = filepath.Join(os.Getenv("HOME"), ".kademlia-dfs", "storage")
	}

	return &cfg, nil
}

// Save saves configuration to file
func Save(cfg *Config) error {
	configPath := getConfigPath()

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// getConfigPath returns the path to the config file
func getConfigPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE") // Windows
	}
	return filepath.Join(home, ".kademlia-dfs", "config.json")
}

