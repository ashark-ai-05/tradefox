package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func tempSettingsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "settings.json")
}

func TestManager_LoadDefaults(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	if err := mgr.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg := mgr.GetServerConfig()
	if cfg.HTTPPort != 8080 {
		t.Fatalf("expected default HTTPPort 8080, got %d", cfg.HTTPPort)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default LogLevel 'info', got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "logs/visualhft.log" {
		t.Fatalf("expected default LogFile 'logs/visualhft.log', got %q", cfg.LogFile)
	}

	ids := mgr.GetAllPluginIDs()
	if len(ids) != 0 {
		t.Fatalf("expected no plugin IDs, got %v", ids)
	}
}

func TestManager_SaveAndLoad(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	mgr.SetServerConfig(ServerConfig{
		HTTPPort: 9090,
		LogLevel: "debug",
		LogFile:  "logs/custom.log",
	})

	type pluginCfg struct {
		Endpoint string `json:"endpoint"`
	}
	if err := mgr.SetPluginSettings("test-plugin", pluginCfg{Endpoint: "ws://localhost"}); err != nil {
		t.Fatalf("SetPluginSettings error: %v", err)
	}

	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Create a new manager and load from the same file.
	mgr2 := NewManager(fp)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg := mgr2.GetServerConfig()
	if cfg.HTTPPort != 9090 {
		t.Fatalf("expected HTTPPort 9090, got %d", cfg.HTTPPort)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected LogLevel 'debug', got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "logs/custom.log" {
		t.Fatalf("expected LogFile 'logs/custom.log', got %q", cfg.LogFile)
	}

	var pc pluginCfg
	if err := mgr2.GetPluginSettings("test-plugin", &pc); err != nil {
		t.Fatalf("GetPluginSettings error: %v", err)
	}
	if pc.Endpoint != "ws://localhost" {
		t.Fatalf("expected endpoint 'ws://localhost', got %q", pc.Endpoint)
	}
}

func TestManager_PluginSettings(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	type MyPlugin struct {
		Host    string `json:"host"`
		Port    int    `json:"port"`
		Enabled bool   `json:"enabled"`
	}

	original := MyPlugin{
		Host:    "192.168.1.100",
		Port:    5555,
		Enabled: true,
	}

	if err := mgr.SetPluginSettings("my-plugin", original); err != nil {
		t.Fatalf("SetPluginSettings error: %v", err)
	}

	var loaded MyPlugin
	if err := mgr.GetPluginSettings("my-plugin", &loaded); err != nil {
		t.Fatalf("GetPluginSettings error: %v", err)
	}

	if loaded.Host != original.Host {
		t.Fatalf("Host: expected %q, got %q", original.Host, loaded.Host)
	}
	if loaded.Port != original.Port {
		t.Fatalf("Port: expected %d, got %d", original.Port, loaded.Port)
	}
	if loaded.Enabled != original.Enabled {
		t.Fatalf("Enabled: expected %v, got %v", original.Enabled, loaded.Enabled)
	}
}

func TestManager_PluginSettings_NotFound(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	var target json.RawMessage
	err := mgr.GetPluginSettings("nonexistent", &target)
	if err == nil {
		t.Fatal("expected error for unknown plugin ID, got nil")
	}
}

func TestManager_ServerConfig(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	// Verify defaults.
	cfg := mgr.GetServerConfig()
	if cfg.HTTPPort != 8080 {
		t.Fatalf("expected default HTTPPort 8080, got %d", cfg.HTTPPort)
	}

	// Update and verify.
	mgr.SetServerConfig(ServerConfig{
		HTTPPort: 3000,
		LogLevel: "warn",
		LogFile:  "logs/warn.log",
	})

	cfg = mgr.GetServerConfig()
	if cfg.HTTPPort != 3000 {
		t.Fatalf("expected HTTPPort 3000, got %d", cfg.HTTPPort)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected LogLevel 'warn', got %q", cfg.LogLevel)
	}
	if cfg.LogFile != "logs/warn.log" {
		t.Fatalf("expected LogFile 'logs/warn.log', got %q", cfg.LogFile)
	}
}

func TestManager_Backup(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	// First save creates the file (no backup since file doesn't exist yet).
	if err := mgr.Save(); err != nil {
		t.Fatalf("first Save() error: %v", err)
	}

	// Second save should create a backup of the first file.
	mgr.SetServerConfig(ServerConfig{
		HTTPPort: 9999,
		LogLevel: "error",
		LogFile:  "logs/error.log",
	})
	if err := mgr.Save(); err != nil {
		t.Fatalf("second Save() error: %v", err)
	}

	// Look for backup files.
	dir := filepath.Dir(fp)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	backupCount := 0
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > len("settings.json.backup.") &&
			e.Name()[:len("settings.json.backup.")] == "settings.json.backup." {
			backupCount++
		}
	}

	if backupCount == 0 {
		t.Fatal("expected at least one backup file, found none")
	}
}

func TestManager_GetAllPluginIDs(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	plugins := []string{"plugin-alpha", "plugin-beta", "plugin-gamma"}
	for _, id := range plugins {
		if err := mgr.SetPluginSettings(id, map[string]string{"name": id}); err != nil {
			t.Fatalf("SetPluginSettings(%q) error: %v", id, err)
		}
	}

	ids := mgr.GetAllPluginIDs()
	if len(ids) != 3 {
		t.Fatalf("expected 3 plugin IDs, got %d: %v", len(ids), ids)
	}

	// IDs should be sorted.
	expected := []string{"plugin-alpha", "plugin-beta", "plugin-gamma"}
	for i, id := range ids {
		if id != expected[i] {
			t.Fatalf("plugin ID[%d]: expected %q, got %q", i, expected[i], id)
		}
	}
}

func TestManager_ThreadSafety(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	var wg sync.WaitGroup

	// 10 goroutines performing concurrent reads and writes.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				// Write server config.
				mgr.SetServerConfig(ServerConfig{
					HTTPPort: 8080 + n,
					LogLevel: "info",
					LogFile:  "logs/test.log",
				})

				// Read server config.
				_ = mgr.GetServerConfig()

				// Write plugin settings.
				_ = mgr.SetPluginSettings("plugin-"+string(rune('a'+n)), map[string]int{"val": n})

				// Read plugin IDs.
				_ = mgr.GetAllPluginIDs()

				// Read plugin settings (may or may not exist).
				var out map[string]int
				_ = mgr.GetPluginSettings("plugin-"+string(rune('a'+n)), &out)
			}
		}(i)
	}

	wg.Wait()
}

func TestManager_FilePath(t *testing.T) {
	fp := tempSettingsPath(t)
	mgr := NewManager(fp)

	if mgr.FilePath() != fp {
		t.Fatalf("expected FilePath() %q, got %q", fp, mgr.FilePath())
	}
}

func TestManager_DefaultFilePath(t *testing.T) {
	mgr := NewManager("")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	expected := filepath.Join(home, ".visualhft", "settings.json")
	if mgr.FilePath() != expected {
		t.Fatalf("expected default FilePath() %q, got %q", expected, mgr.FilePath())
	}
}
