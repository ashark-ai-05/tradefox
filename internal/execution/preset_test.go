package execution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresetStore_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "presets.json")

	ps, err := NewPresetStore(filePath)
	if err != nil {
		t.Fatalf("failed to create preset store: %v", err)
	}

	// Initially empty
	if len(ps.List()) != 0 {
		t.Fatal("expected empty preset list")
	}

	// Save a preset
	preset := OrderPreset{
		Name:     "scalp-btc",
		Symbol:   "BTCUSDT",
		Side:     "Buy",
		OrderType: "Limit",
		Quantity: 0.1,
		Price:    50000,
		Leverage: 10,
	}

	if err := ps.Save(preset); err != nil {
		t.Fatalf("failed to save preset: %v", err)
	}

	// List should have 1
	if len(ps.List()) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(ps.List()))
	}

	// Get by name
	got, ok := ps.Get("scalp-btc")
	if !ok {
		t.Fatal("expected to find preset by name")
	}
	if got.Quantity != 0.1 {
		t.Fatalf("expected quantity 0.1, got %f", got.Quantity)
	}

	// Update
	preset.Quantity = 0.5
	if err := ps.Save(preset); err != nil {
		t.Fatalf("failed to update preset: %v", err)
	}
	got, _ = ps.Get("scalp-btc")
	if got.Quantity != 0.5 {
		t.Fatalf("expected updated quantity 0.5, got %f", got.Quantity)
	}

	// Delete
	if err := ps.Delete("scalp-btc"); err != nil {
		t.Fatalf("failed to delete preset: %v", err)
	}
	if len(ps.List()) != 0 {
		t.Fatal("expected empty list after delete")
	}

	// Delete nonexistent
	if err := ps.Delete("nonexistent"); err == nil {
		t.Fatal("expected error deleting nonexistent preset")
	}
}

func TestPresetStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "presets.json")

	// Create and save
	ps1, _ := NewPresetStore(filePath)
	ps1.Save(OrderPreset{Name: "test", Symbol: "ETHUSDT", Side: "Sell", Quantity: 5})

	// Reopen
	ps2, err := NewPresetStore(filePath)
	if err != nil {
		t.Fatalf("failed to reopen preset store: %v", err)
	}

	presets := ps2.List()
	if len(presets) != 1 {
		t.Fatalf("expected 1 preset after reload, got %d", len(presets))
	}
	if presets[0].Name != "test" {
		t.Fatalf("expected preset name 'test', got %q", presets[0].Name)
	}
}

func TestPresetStore_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "presets.json")
	ps, _ := NewPresetStore(filePath)

	err := ps.Save(OrderPreset{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestPresetStore_CreatesDirIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "deep", "presets.json")

	ps, err := NewPresetStore(filePath)
	if err != nil {
		t.Fatalf("failed to create preset store with nested dir: %v", err)
	}

	ps.Save(OrderPreset{Name: "test", Symbol: "BTC"})

	// Verify file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("expected file to be created")
	}
}
