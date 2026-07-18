package config

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Config holds all configuration options
type Config struct {
	// Grid settings
	MinCellPx  int    `yaml:"minCellPx"`  // stop zooming when cell width ≤ this
	GridLayout string `yaml:"gridLayout"` // "num" (1-9) or "home" (asdfghjkl)

	// Appearance
	BgColor        string  `yaml:"bgColor"`
	TextColor      string  `yaml:"textColor"`
	HighlightColor string  `yaml:"highlightColor"`
	Opacity        float64 `yaml:"opacity"`

	// Paths
	LogPath    string `yaml:"logPath"`
	ConfigPath string `yaml:"-"` // not in yaml, set by CLI flag

	// Runtime
	Debug bool `yaml:"-"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		MinCellPx:       10,
		GridLayout:      "num",
		BgColor:        "#1a1a2e",
		TextColor:      "#e0e0e0",
		HighlightColor: "#00d4ff",
		Opacity:        0.4,
		LogPath:        filepath.Join(os.Getenv("HOME"), "magnus", "mouseless-grid", "logs"),
	}
}

// Load reads a YAML config file, falling back to defaults for missing fields
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		path = defaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Infof("No config file at %s, using defaults", path)
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	log.Debugf("Loaded config from %s", path)
	return cfg, nil
}

// Save writes the config to a YAML file
func Save(cfg *Config, path string) error {
	if path == "" {
		path = defaultConfigPath()
	}

	// ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// EnsureLogDir creates the log directory if it doesn't exist
func EnsureLogDir(cfg *Config) error {
	return os.MkdirAll(cfg.LogPath, 0755)
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "mouseless-grid", "config.yaml")
}

// ParseColor converts "#RRGGBB" or "#RRGGBBAA" hex string to color.NRGBA
func ParseColor(hex string) color.Color {
	hex = hex[1:] // strip #
	var r, g, b, a uint8 = 0, 0, 0, 255
	fmt.Sscanf(hex, "%02x%02x%02x%02x", &r, &g, &b, &a)
	return color.NRGBA{R: r, G: g, B: b, A: a}
}
