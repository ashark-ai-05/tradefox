package recorder

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RotatingWriter writes JSON records to gzip-compressed JSONL files,
// rotating to a new file every hour.
type RotatingWriter struct {
	dir     string
	prefix  string
	mu      sync.Mutex
	file    *os.File
	gz      *gzip.Writer
	enc     *json.Encoder
	curHour int
	curDay  int
	count   int64
}

// NewRotatingWriter creates a RotatingWriter that writes gzip-compressed JSONL
// files into dir, using the given prefix for filenames. The directory is created
// if it does not already exist.
func NewRotatingWriter(dir, prefix string) (*RotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("recorder: mkdir %s: %w", dir, err)
	}
	return &RotatingWriter{
		dir:     dir,
		prefix:  prefix,
		curHour: -1, // force rotation on first Write
		curDay:  -1,
	}, nil
}

// Write marshals v as a JSON line and writes it to the current file.
// If the UTC hour has changed since the last write, the current file is
// closed and a new one is opened. Write is safe for concurrent use.
func (w *RotatingWriter) Write(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	hour := now.Hour()
	day := now.YearDay()

	if hour != w.curHour || day != w.curDay {
		if err := w.rotateLocked(now); err != nil {
			return err
		}
	}

	if err := w.enc.Encode(v); err != nil {
		return fmt.Errorf("recorder: encode: %w", err)
	}
	w.count++
	return nil
}

// WriteAt marshals v as a JSON line and writes it to the file for time t.
// Used by the backfiller to write historical data to the correct hourly file.
func (w *RotatingWriter) WriteAt(v any, t time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	t = t.UTC()
	hour := t.Hour()
	day := t.YearDay()

	if hour != w.curHour || day != w.curDay {
		if err := w.rotateLocked(t); err != nil {
			return err
		}
	}

	if err := w.enc.Encode(v); err != nil {
		return fmt.Errorf("recorder: encode: %w", err)
	}
	w.count++
	return nil
}

// Close flushes the gzip stream and closes the underlying file.
// It is safe to call Close on a writer that was never written to.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeLocked()
}

// Count returns the number of records written to the current file.
func (w *RotatingWriter) Count() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.count
}

// filename returns the path for the given UTC time:
// {prefix}_{YYYY-MM-DD_HH}.jsonl.gz
func (w *RotatingWriter) filename(t time.Time) string {
	name := fmt.Sprintf("%s_%s.jsonl.gz", w.prefix, t.Format("2006-01-02_15"))
	return filepath.Join(w.dir, name)
}

// rotateLocked closes any open file and opens a new one for the given time.
// Caller must hold w.mu.
func (w *RotatingWriter) rotateLocked(t time.Time) error {
	if err := w.closeLocked(); err != nil {
		return err
	}

	path := w.filename(t)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("recorder: open %s: %w", path, err)
	}

	w.file = f
	w.gz = gzip.NewWriter(f)
	w.enc = json.NewEncoder(w.gz)
	w.curHour = t.Hour()
	w.curDay = t.YearDay()
	w.count = 0
	return nil
}

// closeLocked flushes and closes the gzip writer and file.
// Caller must hold w.mu.
func (w *RotatingWriter) closeLocked() error {
	if w.gz != nil {
		if err := w.gz.Close(); err != nil {
			return fmt.Errorf("recorder: gzip close: %w", err)
		}
		w.gz = nil
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("recorder: file close: %w", err)
		}
		w.file = nil
	}
	w.enc = nil
	return nil
}
