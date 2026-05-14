package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all runtime configuration for CoworkAgent.
type Config struct {
	// LLM settings
	OllamaBaseURL string `json:"ollama_base_url"`
	DefaultModel  string `json:"default_model"`
	MaxTokens     int    `json:"max_tokens"`
	ContextWindow int    `json:"context_window"`

	// Indexer settings
	DBPath        string   `json:"db_path"`
	IgnoredDirs   []string `json:"ignored_dirs"`
	SupportedExts []string `json:"supported_exts"`

	// Thermal throttling
	ThermalThresholdCelsius float64 `json:"thermal_threshold_celsius"`
	CPUThrottlePercent      float64 `json:"cpu_throttle_percent"`
	ThrottleDelayMs         int     `json:"throttle_delay_ms"`

	// Shadow workspace
	BranchPrefix string `json:"branch_prefix"`

	// Termux
	TermuxNotifyEnabled bool `json:"termux_notify_enabled"`

	// Project
	ProjectRoot string `json:"project_root"`
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		OllamaBaseURL:           "http://localhost:11434",
		DefaultModel:            "qwen2.5-coder:7b",
		MaxTokens:               2048,
		ContextWindow:           4096,
		DBPath:                  filepath.Join(home, ".local", "share", "cowork-agent", "index.db"),
		IgnoredDirs:             []string{".git", "node_modules", "vendor", "__pycache__", ".cache", "dist", "build"},
		SupportedExts:           []string{".go", ".py", ".ts", ".js", ".lua", ".rs", ".cpp", ".c", ".h", ".java", ".md", ".yaml", ".toml", ".json"},
		ThermalThresholdCelsius: 42.0,
		CPUThrottlePercent:      75.0,
		ThrottleDelayMs:         500,
		BranchPrefix:            "cowork",
		TermuxNotifyEnabled:     true,
		ProjectRoot:             ".",
	}
}

// Load reads config from ~/.config/cowork-agent/config.json,
// falling back to defaults for any missing field.
func Load() (*Config, error) {
	cfg := defaults()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	cfgPath := filepath.Join(home, ".config", "cowork-agent", "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Write defaults for first-time users
			_ = save(cfg, cfgPath)
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
