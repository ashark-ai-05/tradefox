package replay

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

// Record is a union type for replayed records.
type Record struct {
	LocalTS int64
	Type    string
	OB      *recorder.OrderBookRecord
	Trade   *recorder.TradeRecord
	Kiy     *recorder.KiyotakaRecord
}

// ReadDir reads all .jsonl.gz files from a directory and returns records
// sorted by local timestamp.
func ReadDir(dir string) ([]Record, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl.gz"))
	if err != nil {
		return nil, fmt.Errorf("replay: glob %s: %w", dir, err)
	}

	var records []Record
	for _, path := range matches {
		recs, err := readFile(path)
		if err != nil {
			return nil, fmt.Errorf("replay: read %s: %w", path, err)
		}
		records = append(records, recs...)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].LocalTS < records[j].LocalTS
	})

	return records, nil
}

// ReadAllData reads orderbooks, trades, and kiyotaka data from a recording
// directory and returns them merged and sorted by timestamp.
func ReadAllData(baseDir string) ([]Record, error) {
	var all []Record

	obDir := filepath.Join(baseDir, "orderbooks")
	if recs, err := ReadDir(obDir); err == nil {
		all = append(all, recs...)
	}

	trDir := filepath.Join(baseDir, "trades")
	if recs, err := ReadDir(trDir); err == nil {
		all = append(all, recs...)
	}

	kiyDir := filepath.Join(baseDir, "kiyotaka")
	if recs, err := ReadDir(kiyDir); err == nil {
		all = append(all, recs...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].LocalTS < all[j].LocalTS
	})

	return all, nil
}

func readFile(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var records []Record
	scanner := bufio.NewScanner(gz)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		rec, err := parseRecord(line)
		if err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

func parseRecord(line []byte) (Record, error) {
	var probe struct {
		Type    string `json:"type"`
		LocalTS int64  `json:"local_ts"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return Record{}, err
	}

	r := Record{LocalTS: probe.LocalTS, Type: probe.Type}

	switch probe.Type {
	case "orderbook":
		var ob recorder.OrderBookRecord
		if err := json.Unmarshal(line, &ob); err != nil {
			return Record{}, err
		}
		r.OB = &ob
	case "trade":
		var tr recorder.TradeRecord
		if err := json.Unmarshal(line, &tr); err != nil {
			return Record{}, err
		}
		r.Trade = &tr
	case "oi", "funding", "liquidation", "ohlcv":
		var kiy recorder.KiyotakaRecord
		if err := json.Unmarshal(line, &kiy); err != nil {
			return Record{}, err
		}
		r.Kiy = &kiy
	default:
		return Record{}, fmt.Errorf("unknown type: %s", probe.Type)
	}

	return r, nil
}
