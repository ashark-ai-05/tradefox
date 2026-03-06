package live

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/eventbus"
	"github.com/ashark-ai-05/tradefox/internal/scanner"
)

// ScannerBridge runs the scanner engine in a background goroutine
// and feeds results to the TUI scanner view.
type ScannerBridge struct {
	engine  *scanner.ScannerEngine
	results chan []scanner.CoinScan
	mu      sync.RWMutex
	latest  []scanner.CoinScan
}

// NewScannerBridge creates a scanner bridge.
// If bus or logger is nil, the bridge operates in a degraded mode.
func NewScannerBridge(bus *eventbus.Bus, logger *slog.Logger) *ScannerBridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScannerBridge{
		engine:  scanner.NewScannerEngine(bus, logger),
		results: make(chan []scanner.CoinScan, 1),
	}
}

// Start begins the scanner loop. It runs scan cycles and pushes results.
func (s *ScannerBridge) Start(ctx context.Context) {
	// Start the scanner engine's internal loop
	s.engine.Start(ctx)

	// Poll for results periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			results := s.engine.GetResults()
			if len(results) > 0 {
				s.mu.Lock()
				s.latest = results
				s.mu.Unlock()

				// Non-blocking send
				select {
				case s.results <- results:
				default:
				}
			}
		}
	}
}

// Results returns a channel that receives scanner results.
func (s *ScannerBridge) Results() <-chan []scanner.CoinScan {
	return s.results
}

// Latest returns the most recent scan results.
func (s *ScannerBridge) Latest() []scanner.CoinScan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]scanner.CoinScan, len(s.latest))
	copy(out, s.latest)
	return out
}

// UpdateConfig updates the scanner engine configuration.
func (s *ScannerBridge) UpdateConfig(cfg scanner.ScannerConfig) {
	s.engine.UpdateConfig(cfg)
}
