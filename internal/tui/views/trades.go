package views

import (
	"fmt"
	"strings"

	"github.com/ashark-ai-05/tradefox/internal/tui/live"
	"github.com/ashark-ai-05/tradefox/internal/tui/mock"
	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// TradesView displays a scrolling recent trades feed.
type TradesView struct {
	Trades     []mock.Trade
	LiveTrades []live.TradeEvent
	UseLive    bool
	Width      int
	Height     int
	Theme      theme.Theme
}

// NewTradesView creates a new trades feed.
func NewTradesView(t theme.Theme) TradesView {
	return TradesView{
		Trades:     mock.GenerateMockTrades(50),
		LiveTrades: make([]live.TradeEvent, 0, 100),
		Theme:      t,
	}
}

// AddLiveTrade appends a live trade event, keeping a ring buffer of 100.
func (tv *TradesView) AddLiveTrade(trade live.TradeEvent) {
	tv.UseLive = true
	tv.LiveTrades = append([]live.TradeEvent{trade}, tv.LiveTrades...)
	if len(tv.LiveTrades) > 100 {
		tv.LiveTrades = tv.LiveTrades[:100]
	}
}

// SetSize updates the trades view dimensions.
func (tv *TradesView) SetSize(w, h int) {
	tv.Width = w
	tv.Height = h
}

// View renders the recent trades feed.
func (tv TradesView) View() string {
	t := tv.Theme
	w := tv.Width
	if w < 20 {
		w = 35
	}
	h := tv.Height
	if h < 5 {
		h = 15
	}
	innerW := w - 4

	liveTag := ""
	if tv.UseLive && len(tv.LiveTrades) > 0 {
		liveTag = " [LIVE]"
	}
	title := t.TableHeader.Render(centerPad("Recent Trades"+liveTag, innerW))

	header := fmt.Sprintf(" %-8s %10s %10s %4s", "Time", "Price", "Size", "Side")
	headerLine := t.TableHeader.Render(truncOrPad(header, innerW))

	var lines []string
	lines = append(lines, title)
	lines = append(lines, headerLine)

	maxRows := h - 4
	if maxRows < 1 {
		maxRows = 1
	}

	if tv.UseLive && len(tv.LiveTrades) > 0 {
		for i, trade := range tv.LiveTrades {
			if i >= maxRows {
				break
			}

			timeStr := trade.Time.Format("15:04:05")

			var priceFmt string
			if trade.Price >= 100 {
				priceFmt = fmt.Sprintf("%.2f", trade.Price)
			} else {
				priceFmt = fmt.Sprintf("%.4f", trade.Price)
			}

			var sizeFmt string
			if trade.Size >= 1.0 {
				sizeFmt = fmt.Sprintf("%.3f", trade.Size)
			} else {
				sizeFmt = fmt.Sprintf("%.4f", trade.Size)
			}

			line := fmt.Sprintf(" %-8s %10s %10s %4s", timeStr, priceFmt, sizeFmt, trade.Side)

			// Large trade: >1 BTC for BTCUSDT, or >50k USD notional
			notional := trade.Price * trade.Size
			isLarge := trade.Size >= 1.0 || notional >= 50000
			style := t.PriceUp
			if trade.Side == "SELL" {
				style = t.PriceDown
			}
			if isLarge {
				style = style.Bold(true)
			}

			lines = append(lines, style.Render(truncOrPad(line, innerW)))
		}
	} else {
		for i, trade := range tv.Trades {
			if i >= maxRows {
				break
			}

			minutes := trade.Time / 60
			seconds := trade.Time % 60
			timeStr := fmt.Sprintf("%02d:%02d:%02d", minutes/60, minutes%60, seconds)

			var priceFmt string
			if trade.Price >= 100 {
				priceFmt = fmt.Sprintf("%.2f", trade.Price)
			} else {
				priceFmt = fmt.Sprintf("%.4f", trade.Price)
			}

			var sizeFmt string
			if trade.Size >= 1.0 {
				sizeFmt = fmt.Sprintf("%.3f", trade.Size)
			} else {
				sizeFmt = fmt.Sprintf("%.4f", trade.Size)
			}

			line := fmt.Sprintf(" %-8s %10s %10s %4s", timeStr, priceFmt, sizeFmt, trade.Side)

			isLarge := trade.Size >= 1.0
			style := t.PriceUp
			if trade.Side == "SELL" {
				style = t.PriceDown
			}
			if isLarge {
				style = style.Bold(true)
			}

			lines = append(lines, style.Render(truncOrPad(line, innerW)))
		}
	}

	content := strings.Join(lines, "\n")

	return t.PanelInactive.
		Width(w - 2).
		Height(h).
		Render(content)
}
