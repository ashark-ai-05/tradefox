package validate

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/replay"
)

// ValidationReport holds all validation results.
type ValidationReport struct {
	Period         string              `json:"period"`
	Symbols        []string            `json:"symbols"`
	TotalRecords   int64               `json:"totalRecords"`
	TotalSnapshots int                 `json:"totalSnapshots"`
	SignalResults  []SignalReport      `json:"signalResults"`
	Interactions   []InteractionResult `json:"interactions"`
	RegimeResults  []RegimeResult      `json:"regimeResults"`
	KilledSignals  []string            `json:"killedSignals"`
}

// SignalReport has per-signal validation metrics.
type SignalReport struct {
	Name        string                        `json:"name"`
	DecayCurve  map[string]float64            `json:"decayCurve"`
	PeakHorizon string                        `json:"peakHorizon"`
	HitRates    map[string]map[string]float64 `json:"hitRates"` // threshold -> horizon -> hit rate
	Alive       bool                          `json:"alive"`
	KillReason  string                        `json:"killReason,omitempty"`
}

// GenerateReport produces a complete validation report from return rows.
func GenerateReport(rows []ReturnRow, records []replay.Record) *ValidationReport {
	extractors := AllExtractors()
	horizons := DefaultHorizons
	thresholds := []float64{0.05, 0.10, 0.15, 0.20, 0.30}

	// Collect symbols
	symbolSet := map[string]bool{}
	for _, row := range rows {
		symbolSet[row.Symbol] = true
	}
	symbols := make([]string, 0, len(symbolSet))
	for s := range symbolSet {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	// Per-signal analysis
	var signalResults []SignalReport
	var killedSignals []string

	for name, extract := range extractors {
		// Decay curve
		curve := DecayCurve(rows, extract, horizons)
		curveStr := map[string]float64{}
		var peakCorr float64
		var peakHorizon string
		for h, c := range curve {
			key := h.String()
			curveStr[key] = c
			if !math.IsNaN(c) && math.Abs(c) > math.Abs(peakCorr) {
				peakCorr = c
				peakHorizon = key
			}
		}

		// Hit rates at various thresholds
		hitRates := map[string]map[string]float64{}
		alive := false
		for _, thresh := range thresholds {
			threshKey := fmt.Sprintf("%.2f", thresh)
			hitRates[threshKey] = map[string]float64{}
			for _, h := range horizons {
				hr, n := HitRate(rows, extract, thresh, h)
				hitRates[threshKey][h.String()] = hr
				if n >= 30 && hr >= 0.52 {
					alive = true // signal passes kill criterion at at least one threshold/horizon
				}
			}
		}

		killReason := ""
		if !alive {
			killReason = "hit rate < 52% at all thresholds and horizons"
			killedSignals = append(killedSignals, name)
		}

		signalResults = append(signalResults, SignalReport{
			Name:        name,
			DecayCurve:  curveStr,
			PeakHorizon: peakHorizon,
			HitRates:    hitRates,
			Alive:       alive,
			KillReason:  killReason,
		})
	}

	// Sort signal results by name for consistent output
	sort.Slice(signalResults, func(i, j int) bool {
		return signalResults[i].Name < signalResults[j].Name
	})

	// Interactions
	interactions := ComputeInteractions(rows)

	// Regime metrics (for key signals only)
	var regimeResults []RegimeResult
	keySignals := []string{"ofi", "microprice", "depth", "composite"}
	for _, name := range keySignals {
		if ext, ok := extractors[name]; ok {
			regimeResults = append(regimeResults, ComputeRegimeMetrics(rows, name, ext, horizons)...)
		}
	}

	// Period
	period := ""
	if len(rows) > 0 {
		start := time.UnixMilli(rows[0].Timestamp).Format("2006-01-02")
		end := time.UnixMilli(rows[len(rows)-1].Timestamp).Format("2006-01-02")
		period = start + " -> " + end
	}

	return &ValidationReport{
		Period:         period,
		Symbols:        symbols,
		TotalRecords:   int64(len(records)),
		TotalSnapshots: len(rows),
		SignalResults:  signalResults,
		Interactions:   interactions,
		RegimeResults:  regimeResults,
		KilledSignals:  killedSignals,
	}
}

// WriteJSON writes the report as JSON.
func (r *ValidationReport) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteSummary writes a human-readable text summary.
func (r *ValidationReport) WriteSummary(w io.Writer) {
	fmt.Fprintf(w, "===================================================\n")
	fmt.Fprintf(w, " SIGNAL VALIDATION REPORT\n")
	fmt.Fprintf(w, "===================================================\n")
	fmt.Fprintf(w, " Period:     %s\n", r.Period)
	fmt.Fprintf(w, " Symbols:    %v\n", r.Symbols)
	fmt.Fprintf(w, " Records:    %d\n", r.TotalRecords)
	fmt.Fprintf(w, " Snapshots:  %d\n", r.TotalSnapshots)
	fmt.Fprintf(w, "---------------------------------------------------\n")

	for _, sig := range r.SignalResults {
		status := "ALIVE"
		if !sig.Alive {
			status = "KILLED"
		}
		fmt.Fprintf(w, "\n %-12s [%s]\n", sig.Name, status)
		fmt.Fprintf(w, "   Peak horizon: %s\n", sig.PeakHorizon)
		if sig.KillReason != "" {
			fmt.Fprintf(w, "   Kill reason:  %s\n", sig.KillReason)
		}
		// Show decay curve
		fmt.Fprintf(w, "   Decay curve:\n")
		for _, h := range DefaultHorizons {
			if c, ok := sig.DecayCurve[h.String()]; ok && !math.IsNaN(c) {
				fmt.Fprintf(w, "     %-6s  %.4f\n", h.String(), c)
			}
		}
	}

	if len(r.KilledSignals) > 0 {
		fmt.Fprintf(w, "\n KILLED SIGNALS: %v\n", r.KilledSignals)
	}

	fmt.Fprintf(w, "\n INTERACTIONS:\n")
	for _, inter := range r.Interactions {
		fmt.Fprintf(w, "   %s + %s -> hit rate: %.1f%% (n=%d, lift: %+.1f%%)\n",
			inter.Signal1, inter.Signal2, inter.HitRate*100, inter.N, inter.Lift*100)
	}

	fmt.Fprintf(w, "===================================================\n")
}
