package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ReplayPath    string `json:"replay_path"`
	SetupComplete bool   `json:"setup_complete"`

	cfgPath string `json:"-"`
}

// DefaultReplayPath returns the default SC1 Remastered replay directory.
func DefaultReplayPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "StarCraft", "maps", "replays")
}

// Load reads config from disk, then overrides with environment variables.
// Call this after godotenv.Load() so env vars are already set.
func Load(cfgPath string) (*Config, error) {
	cfg := &Config{
		ReplayPath:    DefaultReplayPath(),
		SetupComplete: false,
		cfgPath:       cfgPath,
	}

	// config.json 로드 (없으면 기본값 사용)
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}
	cfg.cfgPath = cfgPath

	// 환경변수 우선 적용
	if v := os.Getenv("REPLAY_PATH"); v != "" {
		cfg.ReplayPath = v
	}

	return cfg, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.cfgPath, data, 0644)
}
