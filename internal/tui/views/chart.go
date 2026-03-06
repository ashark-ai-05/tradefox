package views

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// Timeframes available for the chart.
var Timeframes = []string{"1m", "5m", "15m", "1h", "4h", "1D"}

// ChartView renders a braille candlestick chart with EMA overlays.
type ChartView struct {
	Symbol         string
	TimeframeIdx   int
	Candles        []mock.Candle
	Width          int
	Height         int
	Theme          theme.Theme
	CrosshairOn    bool
	CrosshairPos   int
}

// NewChartView creates a new chart view.
func NewChartView(t theme.Theme) ChartView {
	cv := ChartView{
		Symbol:       "BTCUSDT",
		TimeframeIdx: 3, // 1h default
		Theme:        t,
	}
	cv.LoadCandles()
	return cv
}

// Timeframe returns the current timeframe string.
func (c *ChartView) Timeframe() string {
	return Timeframes[c.TimeframeIdx]
}

// NextTimeframe cycles to the next timeframe.
func (c *ChartView) NextTimeframe() {
	c.TimeframeIdx = (c.TimeframeIdx + 1) % len(Timeframes)
	c.LoadCandles()
}

// PrevTimeframe cycles to the previous timeframe.
func (c *ChartView) PrevTimeframe() {
	c.TimeframeIdx = (c.TimeframeIdx + len(Timeframes) - 1) % len(Timeframes)
	c.LoadCandles()
}

// LoadCandles loads candle data for current symbol/timeframe.
func (c *ChartView) LoadCandles() {
	c.Candles = mock.GenerateMockCandles(c.Symbol, c.Timeframe(), 120)
	if c.CrosshairPos >= len(c.Candles) {
		c.CrosshairPos = len(c.Candles) - 1
	}
}

// ema calculates exponential moving average.
func ema(candles []mock.Candle, period int) []float64 {
	result := make([]float64, len(candles))
	if len(candles) == 0 {
		return result
	}
	k := 2.0 / (float64(period) + 1.0)
	result[0] = candles[0].Close
	for i := 1; i < len(candles); i++ {
		result[i] = candles[i].Close*k + result[i-1]*(1.0-k)
	}
	return result
}

// View renders the chart.
func (c ChartView) View() string {
	t := c.Theme
	w := c.Width
	if w < 30 {
		w = 80
	}
	h := c.Height
	if h < 10 {
		h = 20
	}

	innerW := w - 4
	innerH := h - 2

	// Reserve space: 1 line title, 1 line timeframe bar, 3 lines volume, 1 line x-axis
	chartH := innerH - 6
	if chartH < 4 {
		chartH = 4
	}
	volH := 3

	// Y-axis label width
	yLabelW := 10
	chartW := innerW - yLabelW - 1
	if chartW < 10 {
		chartW = 10
	}

	// How many candles to show (1 char per candle)
	numCandles := chartW
	if numCandles > len(c.Candles) {
		numCandles = len(c.Candles)
	}
	startIdx := len(c.Candles) - numCandles
	visibleCandles := c.Candles[startIdx:]

	// Compute EMAs
	ema9 := ema(c.Candles, 9)
	ema21 := ema(c.Candles, 21)
	visEma9 := ema9[startIdx:]
	visEma21 := ema21[startIdx:]

	// Find price range
	minPrice := math.MaxFloat64
	maxPrice := -math.MaxFloat64
	for _, candle := range visibleCandles {
		if candle.Low < minPrice {
			minPrice = candle.Low
		}
		if candle.High > maxPrice {
			maxPrice = candle.High
		}
	}
	for i := range visibleCandles {
		if visEma9[i] < minPrice {
			minPrice = visEma9[i]
		}
		if visEma9[i] > maxPrice {
			maxPrice = visEma9[i]
		}
		if visEma21[i] < minPrice {
			minPrice = visEma21[i]
		}
		if visEma21[i] > maxPrice {
			maxPrice = visEma21[i]
		}
	}
	padding := (maxPrice - minPrice) * 0.05
	minPrice -= padding
	maxPrice += padding
	priceRange := maxPrice - minPrice
	if priceRange == 0 {
		priceRange = 1
	}

	// Sub-pixel grid using braille: each char = 2 cols x 4 rows of dots
	// But for simplicity with 1 char per candle, we use block chars for the candle body
	// and braille for wicks and EMA lines

	// Build the chart as a grid of styled characters
	// Each row maps to a price range
	grid := make([][]string, chartH)
	for row := range grid {
		grid[row] = make([]string, chartW)
		for col := range grid[row] {
			grid[row][col] = " "
		}
	}

	// Map price to row (0 = top = max price, chartH-1 = bottom = min price)
	priceToRow := func(p float64) int {
		row := int((maxPrice - p) / priceRange * float64(chartH-1))
		if row < 0 {
			row = 0
		}
		if row >= chartH {
			row = chartH - 1
		}
		return row
	}

	// Draw EMA lines first (background)
	emaStyle9 := lipgloss.NewStyle().Foreground(lipgloss.Color("#00bcd4"))  // cyan
	emaStyle21 := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffc107")) // yellow

	for i := 0; i < numCandles && i < chartW; i++ {
		row21 := priceToRow(visEma21[i])
		if grid[row21][i] == " " {
			grid[row21][i] = emaStyle21.Render("·")
		}
		row9 := priceToRow(visEma9[i])
		if grid[row9][i] == " " {
			grid[row9][i] = emaStyle9.Render("·")
		}
	}

	// Draw candles
	for i, candle := range visibleCandles {
		if i >= chartW {
			break
		}
		highRow := priceToRow(candle.High)
		lowRow := priceToRow(candle.Low)
		openRow := priceToRow(candle.Open)
		closeRow := priceToRow(candle.Close)

		isGreen := candle.Close >= candle.Open
		bodyTop := openRow
		bodyBot := closeRow
		if isGreen {
			bodyTop = closeRow
			bodyBot = openRow
		}

		var candleStyle lipgloss.Style
		var bodyChar, wickChar string
		if isGreen {
			candleStyle = t.PriceUp
			bodyChar = "█"
			wickChar = "│"
		} else {
			candleStyle = t.PriceDown
			bodyChar = "░"
			wickChar = "│"
		}

		// Draw wick above body
		for row := highRow; row < bodyTop; row++ {
			grid[row][i] = candleStyle.Render(wickChar)
		}
		// Draw body
		for row := bodyTop; row <= bodyBot; row++ {
			grid[row][i] = candleStyle.Render(bodyChar)
		}
		// Draw wick below body
		for row := bodyBot + 1; row <= lowRow; row++ {
			grid[row][i] = candleStyle.Render(wickChar)
		}
		// If body is zero height, at least draw one char
		if bodyTop == bodyBot {
			grid[bodyTop][i] = candleStyle.Render(bodyChar)
		}
	}

	// Draw crosshair
	if c.CrosshairOn && c.CrosshairPos >= startIdx && c.CrosshairPos < startIdx+numCandles {
		col := c.CrosshairPos - startIdx
		crossStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff00"))
		crossRow := priceToRow(visibleCandles[col].Close)
		for row := 0; row < chartH; row++ {
			if row != crossRow {
				// Don't overwrite candle body, just add cursor column marker on empty cells
				if grid[row][col] == " " {
					grid[row][col] = crossStyle.Render("┊")
				}
			}
		}
		for cCol := 0; cCol < chartW; cCol++ {
			if cCol != col && grid[crossRow][cCol] == " " {
				grid[crossRow][cCol] = crossStyle.Render("─")
			}
		}
		grid[crossRow][col] = crossStyle.Render("╋")
	}

	// Build output lines
	var lines []string

	// Title
	titleStr := fmt.Sprintf(" %s  %s ", c.Symbol, c.Timeframe())
	ema9Label := emaStyle9.Render("EMA9")
	ema21Label := emaStyle21.Render("EMA21")
	title := t.TableHeader.Render(titleStr) + "  " + ema9Label + "  " + ema21Label
	lines = append(lines, truncOrPad(title, innerW))

	// Timeframe selector bar
	var tfParts []string
	for i, tf := range Timeframes {
		if i == c.TimeframeIdx {
			tfParts = append(tfParts, t.Bright.Render("["+tf+"]"))
		} else {
			tfParts = append(tfParts, t.Dim.Render(" "+tf+" "))
		}
	}
	tfBar := "  < " + strings.Join(tfParts, " ") + " >"
	lines = append(lines, t.Dim.Render(truncOrPad(tfBar, innerW)))

	// Chart rows with Y-axis
	numYLabels := 5
	for row := 0; row < chartH; row++ {
		// Y-axis label
		label := ""
		if chartH > 1 {
			// Show label at evenly spaced rows
			labelInterval := chartH / numYLabels
			if labelInterval < 1 {
				labelInterval = 1
			}
			if row%labelInterval == 0 || row == chartH-1 {
				price := maxPrice - (float64(row)/float64(chartH-1))*priceRange
				if price >= 1000 {
					label = fmt.Sprintf("%9.1f", price)
				} else if price >= 1 {
					label = fmt.Sprintf("%9.2f", price)
				} else {
					label = fmt.Sprintf("%9.4f", price)
				}
			}
		}
		yLabel := t.Dim.Render(fmt.Sprintf("%*s", yLabelW, label))
		rowStr := strings.Join(grid[row], "")
		lines = append(lines, yLabel+"│"+rowStr)
	}

	// Volume bars at bottom
	var maxVol float64
	for _, candle := range visibleCandles {
		if candle.Volume > maxVol {
			maxVol = candle.Volume
		}
	}
	if maxVol == 0 {
		maxVol = 1
	}

	volChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	for row := 0; row < volH; row++ {
		yLabel := t.Dim.Render(fmt.Sprintf("%*s", yLabelW, ""))
		var volRow strings.Builder
		for i, candle := range visibleCandles {
			if i >= chartW {
				break
			}
			ratio := candle.Volume / maxVol
			// Each row covers 1/volH of the volume range
			rowRatio := ratio * float64(volH) - float64(volH-1-row)
			var ch string
			if rowRatio >= 1.0 {
				ch = "█"
			} else if rowRatio > 0 {
				idx := int(rowRatio * float64(len(volChars)))
				if idx >= len(volChars) {
					idx = len(volChars) - 1
				}
				if idx < 0 {
					idx = 0
				}
				ch = string(volChars[idx])
			} else {
				ch = " "
			}
			isGreen := candle.Close >= candle.Open
			if isGreen {
				volRow.WriteString(t.PriceUp.Render(ch))
			} else {
				volRow.WriteString(t.PriceDown.Render(ch))
			}
		}
		lines = append(lines, yLabel+"│"+volRow.String())
	}

	// X-axis labels
	xAxis := strings.Repeat(" ", yLabelW) + "└" + strings.Repeat("─", chartW)
	lines = append(lines, t.Dim.Render(truncOrPad(xAxis, innerW)))

	// Crosshair OHLCV info
	if c.CrosshairOn && c.CrosshairPos >= startIdx && c.CrosshairPos < startIdx+numCandles {
		candle := visibleCandles[c.CrosshairPos-startIdx]
		info := fmt.Sprintf(" O:%.2f H:%.2f L:%.2f C:%.2f V:%s",
			candle.Open, candle.High, candle.Low, candle.Close,
			mock.FormatVolume(candle.Volume))
		crossStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffff00"))
		lines = append(lines, crossStyle.Render(truncOrPad(info, innerW)))
	}

	content := strings.Join(lines, "\n")

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}
