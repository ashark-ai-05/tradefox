package views

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// LiquidationView renders a vertical liquidation heatmap alongside the chart.
type LiquidationView struct {
	Bands        []mock.LiqBand
	CurrentPrice float64
	Width        int
	Height       int
	Visible      bool
	Theme        theme.Theme
}

// NewLiquidationView creates a liquidation heatmap.
func NewLiquidationView(t theme.Theme) LiquidationView {
	price := 67432.50
	return LiquidationView{
		Bands:        mock.GenerateMockLiquidationBands(price, 15),
		CurrentPrice: price,
		Width:        10,
		Visible:      true,
		Theme:        t,
	}
}

// View renders the liquidation heatmap.
func (lv LiquidationView) View() string {
	if !lv.Visible {
		return ""
	}

	t := lv.Theme
	w := lv.Width
	if w < 6 {
		w = 10
	}
	h := lv.Height
	if h < 5 {
		h = 20
	}

	// Sort bands by price descending (top = highest price)
	sorted := make([]mock.LiqBand, len(lv.Bands))
	copy(sorted, lv.Bands)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Price > sorted[j].Price
	})

	// Find price range
	minPrice := math.MaxFloat64
	maxPrice := -math.MaxFloat64
	for _, b := range sorted {
		if b.Price < minPrice {
			minPrice = b.Price
		}
		if b.Price > maxPrice {
			maxPrice = b.Price
		}
	}
	// Ensure current price is in range
	if lv.CurrentPrice < minPrice {
		minPrice = lv.CurrentPrice
	}
	if lv.CurrentPrice > maxPrice {
		maxPrice = lv.CurrentPrice
	}
	padding := (maxPrice - minPrice) * 0.05
	minPrice -= padding
	maxPrice += padding
	priceRange := maxPrice - minPrice
	if priceRange == 0 {
		priceRange = 1
	}

	innerH := h - 2
	if innerH < 3 {
		innerH = 3
	}

	// Build density map per row
	barW := w - 2 // leave room for panel border
	if barW < 3 {
		barW = 3
	}

	// Warm colors for long liquidations (below price): gray → yellow → orange → red
	longColors := []lipgloss.Color{
		"#404040", "#666600", "#996600", "#cc3300", "#ff0000",
	}
	// Cool colors for short liquidations (above price): gray → cyan → blue → purple
	shortColors := []lipgloss.Color{
		"#404040", "#006666", "#0066cc", "#6600cc", "#9900ff",
	}

	var lines []string
	lines = append(lines, t.Dim.Render(centerPad("Liq", barW)))

	for row := 0; row < innerH; row++ {
		rowPrice := maxPrice - (float64(row)/float64(innerH-1))*priceRange

		// Find closest band
		var closestBand *mock.LiqBand
		closestDist := math.MaxFloat64
		for i := range sorted {
			dist := math.Abs(sorted[i].Price - rowPrice)
			if dist < closestDist {
				closestDist = dist
				closestBand = &sorted[i]
			}
		}

		// Current price marker
		curPriceRow := int((maxPrice - lv.CurrentPrice) / priceRange * float64(innerH-1))
		if row == curPriceRow {
			marker := t.Bright.Render(fmt.Sprintf("►%*.0f", barW-1, lv.CurrentPrice))
			lines = append(lines, truncOrPad(marker, barW))
			continue
		}

		// Determine density and color
		density := 0.0
		isShort := rowPrice > lv.CurrentPrice
		if closestBand != nil && closestDist < priceRange/float64(innerH)*1.5 {
			density = closestBand.Density
		}

		// Render density bar
		heatChars := []rune{' ', '░', '▒', '▓', '█'}
		charIdx := int(density * float64(len(heatChars)-1))
		if charIdx >= len(heatChars) {
			charIdx = len(heatChars) - 1
		}

		bar := strings.Repeat(string(heatChars[charIdx]), barW)

		// Color based on side and intensity
		colorIdx := int(density * float64(len(longColors)-1))
		if colorIdx >= len(longColors) {
			colorIdx = len(longColors) - 1
		}
		if colorIdx < 0 {
			colorIdx = 0
		}

		var color lipgloss.Color
		if isShort {
			color = shortColors[colorIdx]
		} else {
			color = longColors[colorIdx]
		}

		style := lipgloss.NewStyle().Foreground(color)
		lines = append(lines, style.Render(bar))
	}

	content := strings.Join(lines, "\n")

	return t.PanelInactive.
		Width(w - 2).
		Height(h).
		Render(content)
}
