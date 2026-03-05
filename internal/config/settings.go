package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RecorderConfig controls the data recorder.
type RecorderConfig struct {
	Enabled bool   `json:"enabled"`
	DataDir string `json:"dataDir"`
}

// ServerConfig holds server-level settings.
type ServerConfig struct {
	HTTPPort int            `json:"httpPort"`
	LogLevel string         `json:"logLevel"`
	LogFile  string         `json:"logFile"`
	Recorder RecorderConfig `json:"recorder"`
}

// Settings is the top-level config structure.
type Settings struct {
	Server  ServerConfig               `json:"server"`
	Plugins map[string]json.RawMessage `json:"plugins"`
}

// Manager handles loading, saving, and accessing settings.
type Manager struct {
	mu       sync.RWMutex
	settings Settings
	filePath string
}

// defaultSettings returns the default settings used when no config file exists.
func defaultSettings() Settings {
	return Settings{
		Server: ServerConfig{
			HTTPPort: 8080,
			LogLevel: "info",
			LogFile:  "logs/visualhft.log",
		},
		Plugins: make(map[string]json.RawMessage),
	}
}

// NewManager creates a settings manager for the given file path.
// If filePath is empty, uses ~/.visualhft/settings.json.
func NewManager(filePath string) *Manager {
	if filePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		filePath = filepath.Join(home, ".visualhft", "settings.json")
	}
	return &Manager{
		settings: defaultSettings(),
		filePath: filePath,
	}
}

// Load reads settings from the JSON file. If the file doesn't exist, uses defaults.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.settings = defaultSettings()
			return nil
		}
		return fmt.Errorf("config: reading settings file: %w", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("config: parsing settings file: %w", err)
	}

	if s.Plugins == nil {
		s.Plugins = make(map[string]json.RawMessage)
	}

	m.settings = s
	return nil
}

// Save writes current settings to the JSON file. Creates a backup first.
func (m *Manager) Save() error {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.settings, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("config: marshalling settings: %w", err)
	}

	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("config: creating settings directory: %w", err)
	}

	// If the file already exists, create a timestamped backup.
	if _, err := os.Stat(m.filePath); err == nil {
		ts := time.Now().Format("20060102_150405")
		backupPath := m.filePath + ".backup." + ts
		existing, err := os.ReadFile(m.filePath)
		if err != nil {
			return fmt.Errorf("config: reading existing settings for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, existing, 0644); err != nil {
			return fmt.Errorf("config: writing backup file: %w", err)
		}
	}

	// Delete backups older than 60 days.
	m.cleanOldBackups()

	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("config: writing settings file: %w", err)
	}
	return nil
}

// cleanOldBackups removes backup files older than 60 days.
func (m *Manager) cleanOldBackups() {
	dir := filepath.Dir(m.filePath)
	base := filepath.Base(m.filePath)
	prefix := base + ".backup."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -60)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// GetServerConfig returns a copy of the server config (thread-safe).
func (m *Manager) GetServerConfig() ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.Server
}

// SetServerConfig updates the server config (thread-safe).
func (m *Manager) SetServerConfig(cfg ServerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.Server = cfg
}

// GetPluginSettings deserializes plugin settings for a given pluginID into target.
func (m *Manager) GetPluginSettings(pluginID string, target any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	raw, ok := m.settings.Plugins[pluginID]
	if !ok {
		return fmt.Errorf("config: plugin settings not found for %q", pluginID)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("config: unmarshalling plugin settings for %q: %w", pluginID, err)
	}
	return nil
}

// SetPluginSettings serializes and stores plugin settings for a given pluginID.
func (m *Manager) SetPluginSettings(pluginID string, settings any) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("config: marshalling plugin settings for %q: %w", pluginID, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.Plugins[pluginID] = raw
	return nil
}

// GetAllPluginIDs returns all configured plugin IDs in sorted order.
func (m *Manager) GetAllPluginIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.settings.Plugins))
	for id := range m.settings.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// FilePath returns the settings file path.
func (m *Manager) FilePath() string {
	return m.filePath
}
