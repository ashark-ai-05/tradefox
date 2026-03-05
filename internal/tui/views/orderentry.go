package views

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ashark-ai-05/tradefox/internal/tui/theme"
)

// OrderType represents the type of order.
type OrderType int

const (
	OrderTypeMarket OrderType = iota
	OrderTypeLimit
	OrderTypeStopLimit
)

func (o OrderType) String() string {
	switch o {
	case OrderTypeMarket:
		return "Market"
	case OrderTypeLimit:
		return "Limit"
	case OrderTypeStopLimit:
		return "Stop-Limit"
	default:
		return "Market"
	}
}

// OrderEntryView is the order entry modal.
type OrderEntryView struct {
	Visible     bool
	IsBuy       bool
	Symbol      string
	OrderType   OrderType
	Price       string
	Quantity    string
	Leverage    string
	StopPrice   string
	ActiveField int
	Confirmed   bool
	Theme       theme.Theme
	Width       int
	Height      int
}

// NewOrderEntryView creates a new order entry view.
func NewOrderEntryView(t theme.Theme) OrderEntryView {
	return OrderEntryView{
		Theme:    t,
		Price:    "67432.50",
		Quantity: "0.01",
		Leverage: "10",
	}
}

// Show makes the modal visible with the given side.
func (o *OrderEntryView) Show(isBuy bool, symbol string, price float64) {
	o.Visible = true
	o.IsBuy = isBuy
	o.Symbol = symbol
	o.Price = fmt.Sprintf("%.2f", price)
	o.Quantity = "0.01"
	o.Leverage = "10"
	o.StopPrice = ""
	o.ActiveField = 0
	o.OrderType = OrderTypeLimit
	o.Confirmed = false
}

// Hide dismisses the modal.
func (o *OrderEntryView) Hide() {
	o.Visible = false
	o.Confirmed = false
}

// NextField moves to the next input field.
func (o *OrderEntryView) NextField() {
	maxField := 3
	if o.OrderType == OrderTypeStopLimit {
		maxField = 4
	}
	o.ActiveField = (o.ActiveField + 1) % (maxField + 1)
}

// PrevField moves to the previous input field.
func (o *OrderEntryView) PrevField() {
	maxField := 3
	if o.OrderType == OrderTypeStopLimit {
		maxField = 4
	}
	o.ActiveField--
	if o.ActiveField < 0 {
		o.ActiveField = maxField
	}
}

// CycleOrderType cycles through order types.
func (o *OrderEntryView) CycleOrderType() {
	o.OrderType = (o.OrderType + 1) % 3
}

// TypeChar adds a character to the active field.
func (o *OrderEntryView) TypeChar(ch string) {
	switch o.ActiveField {
	case 0:
		o.CycleOrderType()
	case 1:
		o.Price += ch
	case 2:
		o.Quantity += ch
	case 3:
		o.Leverage += ch
	case 4:
		o.StopPrice += ch
	}
}

// Backspace removes last char from the active field.
func (o *OrderEntryView) Backspace() {
	switch o.ActiveField {
	case 1:
		if len(o.Price) > 0 {
			o.Price = o.Price[:len(o.Price)-1]
		}
	case 2:
		if len(o.Quantity) > 0 {
			o.Quantity = o.Quantity[:len(o.Quantity)-1]
		}
	case 3:
		if len(o.Leverage) > 0 {
			o.Leverage = o.Leverage[:len(o.Leverage)-1]
		}
	case 4:
		if len(o.StopPrice) > 0 {
			o.StopPrice = o.StopPrice[:len(o.StopPrice)-1]
		}
	}
}

// View renders the order entry modal.
func (o OrderEntryView) View() string {
	if !o.Visible {
		return ""
	}

	t := o.Theme
	modalW := 50

	var sideStr string
	var sideStyle lipgloss.Style
	if o.IsBuy {
		sideStr = "BUY"
		sideStyle = t.PriceUp.Bold(true)
	} else {
		sideStr = "SELL"
		sideStyle = t.PriceDown.Bold(true)
	}

	title := sideStyle.Render(fmt.Sprintf("  %s %s  ", sideStr, o.Symbol))

	fields := []struct {
		label string
		value string
	}{
		{"Type", o.OrderType.String()},
		{"Price", o.Price},
		{"Quantity", o.Quantity},
		{"Leverage", o.Leverage + "x"},
	}
	if o.OrderType == OrderTypeStopLimit {
		fields = append(fields, struct {
			label string
			value string
		}{"Stop Price", o.StopPrice})
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	for i, f := range fields {
		label := t.Dim.Render(fmt.Sprintf("  %-12s", f.label))
		var value string
		if i == o.ActiveField {
			value = t.Bright.Render(fmt.Sprintf(" > %s_ ", f.value))
		} else {
			value = t.Normal.Render(fmt.Sprintf("   %s  ", f.value))
		}
		lines = append(lines, label+value)
	}

	// Risk preview
	lines = append(lines, "")
	price, _ := strconv.ParseFloat(o.Price, 64)
	qty, _ := strconv.ParseFloat(o.Quantity, 64)
	lev, _ := strconv.ParseFloat(o.Leverage, 64)

	if price > 0 && qty > 0 {
		notional := price * qty
		margin := notional / lev
		lines = append(lines, t.Dim.Render("  ─── Risk Preview ───"))
		lines = append(lines, t.Normal.Render(fmt.Sprintf("  Notional:  $%.2f", notional)))
		lines = append(lines, t.Normal.Render(fmt.Sprintf("  Margin:    $%.2f", margin)))
		lines = append(lines, t.PriceUp.Render(fmt.Sprintf("  PnL @+2%%:  $%.2f", notional*0.02)))
		lines = append(lines, t.PriceDown.Render(fmt.Sprintf("  PnL @-2%%: -$%.2f", notional*0.02)))
	}

	lines = append(lines, "")
	if o.Confirmed {
		lines = append(lines, t.Warning.Render("  Order submitted (mock)"))
	} else {
		lines = append(lines, t.Dim.Render("  [Enter] Submit  [Esc] Cancel  [Tab] Next"))
	}

	content := strings.Join(lines, "\n")

	modal := t.ModalBorder.
		Width(modalW).
		Render(content)

	// Center the modal
	screenW := o.Width
	screenH := o.Height
	if screenW < 1 {
		screenW = 120
	}
	if screenH < 1 {
		screenH = 40
	}

	modalH := lipgloss.Height(modal)
	modalActualW := lipgloss.Width(modal)

	padLeft := (screenW - modalActualW) / 2
	padTop := (screenH - modalH) / 3

	if padLeft < 0 {
		padLeft = 0
	}
	if padTop < 0 {
		padTop = 0
	}

	positioned := lipgloss.NewStyle().
		MarginLeft(padLeft).
		MarginTop(padTop).
		Render(modal)

	return positioned
}
