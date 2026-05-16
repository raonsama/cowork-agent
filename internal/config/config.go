// Package config — extended config.go adds fields for:
//   - Think Mode & Plan/Code Mode defaults
//   - Multi-provider LLM (Ollama local + OpenAI-compatible cloud)
//   - Input history persistence opt-in
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all runtime configuration for CoworkAgent.
type Config struct {
	// ── LLM — primary (Ollama / local) ───────────────────
	OllamaBaseURL string `json:"ollama_base_url"`
	DefaultModel  string `json:"default_model"`
	MaxTokens     int    `json:"max_tokens"`
	ContextWindow int    `json:"context_window"`

	// ── LLM — cloud / OpenAI-compatible ──────────────────
	// ProviderKind selects the backend: "ollama" (default) or "openai".
	ProviderKind string `json:"provider_kind"`
	// CloudBaseURL is the base URL for OpenAI-compatible providers.
	// Examples: "https://api.openai.com", "https://api.groq.com/openai",
	//           "https://api.deepseek.com", custom Anthropic proxy.
	CloudBaseURL string `json:"cloud_base_url"`
	// CloudAPIKey is the Bearer token for cloud providers.
	CloudAPIKey string `json:"cloud_api_key"`
	// CloudModel is the model name on the cloud provider.
	CloudModel string `json:"cloud_model"`

	// ── Mode defaults ─────────────────────────────────────
	// ThinkModeDefault enables the reasoning-trace panel on startup.
	ThinkModeDefault bool `json:"think_mode_default"`
	// PlanModeDefault enables Plan/Code Mode on startup.
	PlanModeDefault bool `json:"plan_mode_default"`

	// ── Input history persistence ─────────────────────────
	// PersistHistory saves the input ring-buffer to DBPath on exit
	// and reloads it on next launch.
	PersistHistory bool `json:"persist_history"`
	// HistoryMaxEntries caps the persisted history length.
	HistoryMaxEntries int `json:"history_max_entries"`

	// ── Indexer ───────────────────────────────────────────
	DBPath        string   `json:"db_path"`
	IgnoredDirs   []string `json:"ignored_dirs"`
	SupportedExts []string `json:"supported_exts"`

	// ── Thermal throttling ────────────────────────────────
	ThermalThresholdCelsius float64 `json:"thermal_threshold_celsius"`
	CPUThrottlePercent      float64 `json:"cpu_throttle_percent"`
	ThrottleDelayMs         int     `json:"throttle_delay_ms"`

	// ── Shadow workspace ──────────────────────────────────
	BranchPrefix string `json:"branch_prefix"`

	// ── Termux ────────────────────────────────────────────
	TermuxNotifyEnabled bool `json:"termux_notify_enabled"`

	// ── Project ───────────────────────────────────────────
	ProjectRoot string `json:"project_root"`
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		OllamaBaseURL: "http://127.0.0.1:11434",
		DefaultModel:  "qwen3.5-uncen:2b",
		MaxTokens:     2048,
		ContextWindow: 4096,
		ProviderKind:  "ollama",

		ThinkModeDefault:  false,
		PlanModeDefault:   false,
		PersistHistory:    true,
		HistoryMaxEntries: 200,

		DBPath:        filepath.Join(home, ".local", "share", "cowork-agent", "index.db"),
		IgnoredDirs:   []string{".git", "node_modules", "vendor", "__pycache__", ".cache", "dist", "build"},
		SupportedExts: []string{".go", ".py", ".ts", ".js", ".lua", ".rs", ".cpp", ".c", ".h", ".java", ".md", ".yaml", ".toml", ".json"},

		ThermalThresholdCelsius: 42.0,
		CPUThrottlePercent:      75.0,
		ThrottleDelayMs:         500,
		BranchPrefix:            "cowork",
		TermuxNotifyEnabled:     true,
		ProjectRoot:             ".",
	}
}

// Load reads config from ~/.config/cowork-agent/config.json,
// falling back to defaults for missing fields.
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

// Save persists cfg back to the config file.
func Save(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return save(cfg, filepath.Join(home, ".config", "cowork-agent", "config.json"))
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
