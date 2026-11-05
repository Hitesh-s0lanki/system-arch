package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct{ Address, DataDir, Volume, Image, Network string }

func Load() (Config, error) {
	c := Config{Address: env("LISTEN_ADDR", ":8090"), DataDir: env("DATA_DIR", "/data"), Volume: env("RESULTS_VOLUME", "load-tester-data"), Image: env("K6_IMAGE", "grafana/k6:1.4.2"), Network: env("K6_NETWORK", "load-tester")}
	if !filepath.IsAbs(c.DataDir) {
		return c, fmt.Errorf("DATA_DIR must be absolute")
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
