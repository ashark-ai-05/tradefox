package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/live"
	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// OrderBookView displays the depth ladder.
type OrderBookView struct {
	Bids     []mock.OrderBookLevel
	Asks     []mock.OrderBookLevel
	LiveBids []live.OrderBookLevel
	LiveAsks []live.OrderBookLevel
	UseLive  bool
	Width    int
	Height   int
	Depth    int
	Theme    theme.Theme
}

// NewOrderBookView creates a new order book view with mock data.
func NewOrderBookView(t theme.Theme) OrderBookView {
	bids, asks := mock.OrderBook()
	return OrderBookView{
		Bids:  bids,
		Asks:  asks,
		Depth: 15,
		Theme: t,
	}
}

// SetSize updates the order book dimensions.
func (o *OrderBookView) SetSize(w, h int) {
	o.Width = w
	o.Height = h
}

// View renders the order book depth ladder.
func (o OrderBookView) View() string {
	t := o.Theme
	w := o.Width
	if w < 20 {
		w = 35
	}

	innerW := w - 4

	liveTag := ""
	if o.UseLive && len(o.LiveBids) > 0 {
		liveTag = " [LIVE]"
	}
	title := t.TableHeader.Render(centerPad("Order Book"+liveTag, innerW))

	colHeader := t.Dim.Render(fmt.Sprintf("%-10s %8s %8s %s", "Price", "Qty", "Total", ""))
	if lipgloss.Width(colHeader) > innerW {
		colHeader = colHeader[:innerW]
	}

	// Use live data if available, fallback to mock.
	type obLevel struct {
		Price float64
		Qty   float64
	}
	var bids, asks []obLevel

	if o.UseLive && len(o.LiveBids) > 0 {
		for _, b := range o.LiveBids {
			bids = append(bids, obLevel{b.Price, b.Qty})
		}
		for _, a := range o.LiveAsks {
			asks = append(asks, obLevel{a.Price, a.Qty})
		}
	} else {
		for _, b := range o.Bids {
			bids = append(bids, obLevel{b.Price, b.Qty})
		}
		for _, a := range o.Asks {
			asks = append(asks, obLevel{a.Price, a.Qty})
		}
	}

	// Find max cumulative for bar scaling
	var maxCum float64
	cumAsk := 0.0
	for _, a := range asks {
		cumAsk += a.Qty
		if cumAsk > maxCum {
			maxCum = cumAsk
		}
	}
	cumBid := 0.0
	for _, b := range bids {
		cumBid += b.Qty
		if cumBid > maxCum {
			maxCum = cumBid
		}
	}
	if maxCum == 0 {
		maxCum = 1
	}

	depth := o.Depth
	if depth > len(asks) {
		depth = len(asks)
	}
	if depth > len(bids) {
		depth = len(bids)
	}

	barWidth := 8
	if innerW > 35 {
		barWidth = innerW - 28
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, colHeader)

	// Asks (reversed so highest ask is at top)
	cum := 0.0
	askLines := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		a := asks[depth-1-i]
		cum += a.Qty
		bar := makeBar(cum/maxCum, barWidth)
		line := fmt.Sprintf("%-10.2f %8.4f %8.4f %s", a.Price, a.Qty, cum, bar)
		askLines = append(askLines, t.PriceDown.Render(truncOrPad(line, innerW)))
	}
	lines = append(lines, askLines...)

	// Spread
	spread := 0.0
	if len(asks) > 0 && len(bids) > 0 {
		spread = asks[0].Price - bids[0].Price
	}
	spreadStr := fmt.Sprintf("── Spread: %.2f ──", spread)
	lines = append(lines, t.Warning.Render(centerPad(spreadStr, innerW)))

	// Bids
	cum = 0.0
	for i := 0; i < depth; i++ {
		b := bids[i]
		cum += b.Qty
		bar := makeBar(cum/maxCum, barWidth)
		line := fmt.Sprintf("%-10.2f %8.4f %8.4f %s", b.Price, b.Qty, cum, bar)
		lines = append(lines, t.PriceUp.Render(truncOrPad(line, innerW)))
	}

	content := strings.Join(lines, "\n")

	h := o.Height
	if h < 1 {
		h = depth*2 + 5
	}

	return t.PanelActive.
		Width(w - 2).
		Height(h).
		Render(content)
}

// makeBar creates a bar visualization using block characters.
func makeBar(ratio float64, width int) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	blocks := []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

	filled := ratio * float64(width)
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
		sb.WriteRune(' ')
	}
	return sb.String()
}

func centerPad(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	left := (w - sw) / 2
	right := w - sw - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func truncOrPad(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw > w {
		// Truncate (rune-safe enough for ASCII-heavy content)
		runes := []rune(s)
		if len(runes) > w {
			return string(runes[:w])
		}
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}
