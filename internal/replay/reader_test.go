package replay

import (
	"testing"

	"github.com/ashark-ai-05/tradefox/internal/recorder"
)

func TestReadDir_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	w, err := recorder.NewRotatingWriter(dir, "test")
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}

	for i := 0; i < 5; i++ {
		w.Write(recorder.TradeRecord{
			Type:    "trade",
			Symbol:  "BTC/USDT",
			Price:   "50000",
			Size:    "1.5",
			LocalTS: int64(1000 + i),
		})
	}
	w.Close()

	records, err := ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("got %d records, want 5", len(records))
	}

	for i := 1; i < len(records); i++ {
		if records[i].LocalTS < records[i-1].LocalTS {
			t.Error("records not sorted by timestamp")
		}
	}
}
