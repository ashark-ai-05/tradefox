package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// SignalsView displays the microstructure signal dashboard.
type SignalsView struct {
	Signals mock.SignalSet
	Width   int
	Height  int
	Theme   theme.Theme
}

// NewSignalsView creates a new signals dashboard.
func NewSignalsView(t theme.Theme) SignalsView {
	return SignalsView{
		Signals: mock.GenerateMockSignals(),
		Theme:   t,
	}
}

// View renders the signal dashboard as a 2x4 grid of gauges.
func (sv SignalsView) View() string {
	t := sv.Theme
	w := sv.Width
	if w < 20 {
		w = 40
	}
	h := sv.Height
	if h < 5 {
		h = 12
	}
	innerW := w - 4

	title := t.TableHeader.Render(centerPad("Signals", innerW))

	var lines []string
	lines = append(lines, title)

	signals := sv.Signals.Signals
	// 2x4 grid
	colW := innerW / 2
	if colW < 15 {
		colW = 15
	}

	for row := 0; row < 4; row++ {
		var leftGauge, rightGauge string
		leftIdx := row
		rightIdx := row + 4

		if leftIdx < len(signals) {
			leftGauge = sv.renderGauge(signals[leftIdx], colW-1)
		}
		if rightIdx < len(signals) {
			rightGauge = sv.renderGauge(signals[rightIdx], colW-1)
		}

		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, leftGauge, " ", rightGauge))
	}

	// Composite score (larger display)
	lines = append(lines, t.Dim.Render(strings.Repeat("─", innerW)))
	compositeLabel := "Composite Score"
	score := sv.Signals.CompositeScore
	barW := innerW - len(compositeLabel) - 12
	if barW < 5 {
		barW = 5
	}
	bar := sv.makeGaugeBar(score, barW)
	scoreStyle := sv.stateStyle(sv.Signals.Signals[len(sv.Signals.Signals)-1].State)
	scoreLine := fmt.Sprintf(" %s  %s  %s",
		t.Bright.Render(compositeLabel),
		bar,
		scoreStyle.Render(fmt.Sprintf("%+.2f", score)))
	lines = append(lines, truncOrPad(scoreLine, innerW))

	content := strings.Join(lines, "\n")

	return t.PanelInactive.
		Width(w - 2).
		Height(h).
		Render(content)
}

func (sv SignalsView) renderGauge(sig mock.SignalValue, w int) string {
	t := sv.Theme
	nameW := 12
	barW := w - nameW - 8
	if barW < 3 {
		barW = 3
	}

	name := truncOrPad(sig.Name, nameW)
	bar := sv.makeGaugeBar(sig.Value, barW)
	stateStyle := sv.stateStyle(sig.State)
	valStr := stateStyle.Render(fmt.Sprintf("%+.2f", sig.Value))

	return t.Normal.Render(name) + bar + " " + valStr
}

func (sv SignalsView) makeGaugeBar(value float64, width int) string {
	// Value ranges from -1.0 to 1.0, map to 0..width
	blocks := []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}
	normalized := (value + 1.0) / 2.0 // map to 0..1
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}

	filled := normalized * float64(width)
	full := int(filled)
	frac := filled - float64(full)

	var sb strings.Builder
	for i := 0; i < full && i < width; i++ {
		sb.WriteRune('█')
	}
	if full < width && frac > 0 {
		idx := int(frac * float64(len(blocks)))
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		sb.WriteRune(blocks[idx])
		full++
	}
	for i := full; i < width; i++ {
		sb.WriteRune('░')
	}

	barStr := sb.String()
	if value > 0.2 {
		return sv.Theme.PriceUp.Render(barStr)
	} else if value < -0.2 {
		return sv.Theme.PriceDown.Render(barStr)
	}
	return sv.Theme.Dim.Render(barStr)
}

func (sv SignalsView) stateStyle(state string) lipgloss.Style {
	switch state {
	case "Bullish":
		return sv.Theme.PriceUp
	case "Bearish":
		return sv.Theme.PriceDown
	default:
		return sv.Theme.Dim
	}
}
