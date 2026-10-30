package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// hostName is the native-messaging host id; the extension connects to it by this
// exact name and Chrome looks up <name>.json in its NativeMessagingHosts dir.
const hostName = "io.grove.gw_grab"

// nativeHostManifest is Chrome's native-messaging host manifest schema.
type nativeHostManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// installHost writes the native-messaging host manifest so Chrome can launch
// this binary for the given extension id. The manifest is locked to that one
// extension via allowed_origins.
func installHost(extID string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating gw-grab binary: %w", err)
	}
	exe, _ = filepath.EvalSymlinks(exe)

	manifest := nativeHostManifest{
		Name:           hostName,
		Description:    "gw-grab native host for Grove",
		Path:           exe,
		Type:           "stdio",
		AllowedOrigins: []string{fmt.Sprintf("chrome-extension://%s/", extID)},
	}

	dir, err := manifestDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, hostName+".json")
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[1;32mok:\033[0m Registered native host %s\n  manifest: %s\n  binary:   %s\n  extension: %s\n",
		hostName, path, exe, extID)
	return nil
}

// manifestDir returns Chrome's per-user NativeMessagingHosts directory for the
// current OS. (Chromium/Edge use sibling paths; documented in the README.)
func manifestDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil
	default:
		return "", fmt.Errorf("install-host not supported on %s; see README for manual setup", runtime.GOOS)
	}
}
