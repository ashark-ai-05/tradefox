package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// OrderPreset stores a reusable order configuration.
type OrderPreset struct {
	Name        string  `json:"name"`
	Symbol      string  `json:"symbol"`
	Side        string  `json:"side"`        // "Buy" or "Sell"
	OrderType   string  `json:"orderType"`   // "Market", "Limit", "StopLimit"
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price,omitempty"`
	StopPrice   float64 `json:"stopPrice,omitempty"`
	Leverage    float64 `json:"leverage,omitempty"`
	StopLoss    float64 `json:"stopLoss,omitempty"`
	TakeProfit  float64 `json:"takeProfit,omitempty"`
	TimeInForce string  `json:"timeInForce,omitempty"` // "GTC", "IOC", "FOK"
}

// PresetStore manages CRUD operations for order presets, persisted to a JSON file.
type PresetStore struct {
	mu       sync.RWMutex
	presets  map[string]OrderPreset
	filePath string
}

// NewPresetStore creates a new PresetStore backed by the given file.
func NewPresetStore(filePath string) (*PresetStore, error) {
	ps := &PresetStore{
		presets:  make(map[string]OrderPreset),
		filePath: filePath,
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create preset directory: %w", err)
	}

	// Load existing presets if file exists
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ps, nil // No file yet, start fresh
		}
		return nil, fmt.Errorf("read presets file: %w", err)
	}

	var presets []OrderPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, fmt.Errorf("parse presets file: %w", err)
	}

	for _, p := range presets {
		ps.presets[p.Name] = p
	}

	return ps, nil
}

// List returns all presets.
func (ps *PresetStore) List() []OrderPreset {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]OrderPreset, 0, len(ps.presets))
	for _, p := range ps.presets {
		result = append(result, p)
	}
	return result
}

// Get returns a preset by name.
func (ps *PresetStore) Get(name string) (OrderPreset, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.presets[name]
	return p, ok
}

// Save creates or updates a preset and persists to disk.
func (ps *PresetStore) Save(preset OrderPreset) error {
	if preset.Name == "" {
		return fmt.Errorf("preset name is required")
	}

	ps.mu.Lock()
	ps.presets[preset.Name] = preset
	ps.mu.Unlock()

	return ps.persist()
}

// Delete removes a preset by name and persists to disk.
func (ps *PresetStore) Delete(name string) error {
	ps.mu.Lock()
	if _, ok := ps.presets[name]; !ok {
		ps.mu.Unlock()
		return fmt.Errorf("preset %q not found", name)
	}
	delete(ps.presets, name)
	ps.mu.Unlock()

	return ps.persist()
}

func (ps *PresetStore) persist() error {
	ps.mu.RLock()
	presets := make([]OrderPreset, 0, len(ps.presets))
	for _, p := range ps.presets {
		presets = append(presets, p)
	}
	ps.mu.RUnlock()

	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal presets: %w", err)
	}

	return os.WriteFile(ps.filePath, data, 0o644)
}
