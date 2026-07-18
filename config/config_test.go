package config

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MinCellPx != 10 {
		t.Fatalf("MinCellPx: got %d, want 10", cfg.MinCellPx)
	}
	if cfg.BgColor != "#1a1a2e" {
		t.Fatalf("BgColor: got %q", cfg.BgColor)
	}
	if cfg.Opacity != 0.4 {
		t.Fatalf("Opacity: got %f", cfg.Opacity)
	}
	if cfg.LogPath == "" {
		t.Fatal("LogPath should not be empty")
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if cfg.MinCellPx != 10 {
		t.Fatal("missing file should return defaults")
	}
}

func TestLoadAndSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	saved := &Config{
		MinCellPx:       25,
		BgColor:        "#ff0000",
		TextColor:      "#00ff00",
		HighlightColor: "#0000ff",
		Opacity:        0.8,
		LogPath:        "/tmp/logs",
	}

	if err := Save(saved, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	if loaded.MinCellPx != 25 {
		t.Fatalf("MinCellPx: got %d, want 25", loaded.MinCellPx)
	}
	if loaded.BgColor != "#ff0000" {
		t.Fatalf("BgColor: got %q", loaded.BgColor)
	}
	if loaded.TextColor != "#00ff00" {
		t.Fatalf("TextColor: got %q", loaded.TextColor)
	}
	if loaded.HighlightColor != "#0000ff" {
		t.Fatalf("HighlightColor: got %q", loaded.HighlightColor)
	}
	if loaded.Opacity != 0.8 {
		t.Fatalf("Opacity: got %f", loaded.Opacity)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("invalid: yaml: : :"), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadEmptyFileDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	os.WriteFile(path, []byte{}, 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty file: %v", err)
	}
	if cfg.MinCellPx != 10 {
		t.Fatal("empty file should return defaults")
	}
}

func TestParseColor(t *testing.T) {
	tests := []struct {
		hex          string
		want color.NRGBA
	}{
		{"#ff0000", color.NRGBA{255, 0, 0, 255}},
		{"#00ff00", color.NRGBA{0, 255, 0, 255}},
		{"#0000ff", color.NRGBA{0, 0, 255, 255}},
		{"#000000", color.NRGBA{0, 0, 0, 255}},
		{"#ffffff", color.NRGBA{255, 255, 255, 255}},
		{"#ff000080", color.NRGBA{255, 0, 0, 128}},
		{"#1a1a2e", color.NRGBA{26, 26, 46, 255}},
	}
	for _, tt := range tests {
		c := ParseColor(tt.hex)
		got, ok := c.(color.NRGBA)
		if !ok {
			t.Fatalf("ParseColor(%q) returned %T, want NRGBA", tt.hex, c)
		}
		if got != tt.want {
			t.Errorf("ParseColor(%q) = %#v, want %#v", tt.hex, got, tt.want)
		}
	}
}

func TestEnsureLogDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "logs", "nested")
	cfg := &Config{LogPath: sub}
	if err := EnsureLogDir(cfg); err != nil {
		t.Fatalf("EnsureLogDir: %v", err)
	}
	if _, err := os.Stat(sub); os.IsNotExist(err) {
		t.Fatal("directory was not created")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := defaultConfigPath()
	if path == "" {
		t.Fatal("default config path should not be empty")
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %q", path)
	}
}
