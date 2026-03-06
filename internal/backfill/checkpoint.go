package backfill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CheckpointStore manages backfill progress checkpoints on disk.
type CheckpointStore struct {
	dir string
}

// NewCheckpointStore creates a checkpoint store in the given directory.
func NewCheckpointStore(dir string) (*CheckpointStore, error) {
	cpDir := filepath.Join(dir, ".backfill")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		return nil, fmt.Errorf("checkpoint: mkdir: %w", err)
	}
	return &CheckpointStore{dir: cpDir}, nil
}

// Save writes a checkpoint to disk.
func (s *CheckpointStore) Save(cp Checkpoint) error {
	cp.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(cp.Symbol, cp.DataType)
	return os.WriteFile(path, data, 0o644)
}

// Load reads a checkpoint for the given symbol and data type.
// Returns a zero Checkpoint with no error if the file doesn't exist.
func (s *CheckpointStore) Load(symbol, dataType string) (Checkpoint, error) {
	path := s.path(symbol, dataType)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Checkpoint{}, nil
		}
		return Checkpoint{}, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, err
	}
	return cp, nil
}

func (s *CheckpointStore) path(symbol, dataType string) string {
	return filepath.Join(s.dir, fmt.Sprintf("checkpoint_%s_%s.json", symbol, dataType))
}
