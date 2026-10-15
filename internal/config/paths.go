package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	EnvPathOverride = "DEVBOARD_CONFIG"
	AppDirName      = "devboard"
	ConfigFileName  = "config.toml"
)

// Path: $DEVBOARD_CONFIG, else $XDG_CONFIG_HOME/devboard/config.toml, else ~/.config/devboard/config.toml.
func Path() (string, error) {
	if override := os.Getenv(EnvPathOverride); override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("resolve %s=%q: %w", EnvPathOverride, override, err)
		}
		return abs, nil
	}
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", AppDirName), nil
}

// DataDir holds seen.json. $XDG_DATA_HOME/devboard, else ~/.local/share/devboard.
func DataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, AppDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", AppDirName), nil
}

func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}
