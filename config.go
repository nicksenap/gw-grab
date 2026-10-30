package main

import (
	"os"
	"strconv"
	"time"

	"github.com/nicksenap/gw-grab/internal/llm"
)

// config holds gw-grab settings, sourced from environment variables. Grove
// passes GROVE_DIR/GROVE_CONFIG to plugins; gw-grab keeps its own small set of
// knobs as env vars so it needs no config parser.
//
// There are no API tokens here: page context arrives from the browser extension
// (which is already authenticated to GitHub/Notion/Slack), so the host never
// talks to those services itself.
type config struct {
	model          string
	ollamaEndpoint string
	timeout        time.Duration
	minConfidence  float64
}

func loadConfig() config {
	c := config{
		model:          getenv("GW_GRAB_MODEL", llm.DefaultModel),
		ollamaEndpoint: getenv("GW_GRAB_OLLAMA", llm.DefaultEndpoint),
		timeout:        llm.DefaultTimeout,
		minConfidence:  0.55,
	}
	if v := os.Getenv("GW_GRAB_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			c.timeout = time.Duration(ms) * time.Millisecond
		}
	}
	if v := os.Getenv("GW_GRAB_MIN_CONFIDENCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			c.minConfidence = f
		}
	}
	return c
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
