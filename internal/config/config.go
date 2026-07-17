package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LLMConfig holds the LLM provider connection settings.
type LLMConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

// Targets holds daily nutritional goals.
type Targets struct {
	CaloriesGoal float64 `json:"calories_goal"` // default 2500
	ProteinGoal  float64 `json:"protein_goal"`  // default 120
}

// ScaleConfig holds smart scale connection settings.
type ScaleConfig struct {
	Provider       string `json:"provider"`          // "xiaomi" | ""
	XiaomiUserID   string `json:"xiaomi_user_id"`   // Mi Account email or phone
	XiaomiPassword string `json:"xiaomi_password"`  // supports "env:VAR" ref
}

// BodyConfig holds body measurement related settings.
type BodyConfig struct {
	HeightCm float64    `json:"height_cm"` // used for BMR calculation
	BirthDate string   `json:"birth_date"` // "2006-01-02", for age calculation
	Gender    string   `json:"gender"`     // "male" | "female"
	Scale     ScaleConfig `json:"scale"`
}

// Config is the top-level application configuration.
type Config struct {
	LLM     LLMConfig  `json:"llm"`
	Targets Targets    `json:"targets"`
	Body    BodyConfig `json:"body"`
}

// ConfigPath returns the path to the configuration file: ~/.config/essen/config.json.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "essen", "config.json")
}

// DefaultConfig returns a Config with sensible LLM defaults and fitness targets.
func DefaultConfig() Config {
	return Config{
		LLM: LLMConfig{
			Provider: "auto",
			Model:    "gpt-4o-mini",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "env:OPENAI_API_KEY",
		},
		Targets: Targets{
			CaloriesGoal: 2500,
			ProteinGoal:  120,
		},
		Body: BodyConfig{
			HeightCm: 0,
			Gender:   "male",
			Scale: ScaleConfig{
				Provider: "",
			},
		},
	}
}

// Load reads the configuration from disk. If the file does not exist,
// the default configuration is returned.
func Load() (Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return cfg, nil
}

// Save writes the configuration to disk, creating parent directories as needed.
func Save(cfg Config) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// Write with 0600: config contains API key.
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	// Ensure permissions even when overwriting an existing file.
	os.Chmod(path, 0600)
	return nil
}

// ResolveAPIKey expands "env:VAR_NAME" references to the corresponding
// environment variable value. Other strings are returned unchanged.
func ResolveAPIKey(raw string) string {
	if strings.HasPrefix(raw, "env:") {
		return os.Getenv(raw[4:])
	}
	return raw
}
