package ws

// WSMessage is the JSON envelope for all WebSocket messages.
type WSMessage struct {
	Type     string      `json:"type"`               // "orderbook", "trade", "study", "provider", "position", "signals"
	Symbol   string      `json:"symbol,omitempty"`    // trading symbol (e.g. "BTCUSD")
	Provider string      `json:"provider,omitempty"`  // provider name
	Name     string      `json:"name,omitempty"`      // study/indicator name
	Data     interface{} `json:"data"`                // payload (OrderBook, Trade, etc.)
}
