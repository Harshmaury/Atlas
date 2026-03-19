// @atlas-project: atlas
// @atlas-path: internal/config/env.go
// Package config provides environment variable helpers and path utilities.
// Mirrors the pattern from Nexus internal/config/env.go for consistency.
package config

import (
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultHTTPAddr    = "127.0.0.1:8081"
	DefaultWorkspace   = "~/workspace"
	DefaultNexusAddr   = "http://127.0.0.1:8080"
	DefaultDBPath      = "~/.nexus/atlas.db"
)

func EnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func DurationEnvOrDefault(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func ExpandHome(path string) string {
	if len(path) < 2 || path[:2] != "~/" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
