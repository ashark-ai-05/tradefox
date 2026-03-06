package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	connector "github.com/ashark-ai-05/tradefox/internal/connector"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// envelope is the wire format for messages received from the WebSocket
// server. The Type field determines how Data is deserialized.
type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// WebSocketConnector is the simplest VisualHFT connector. It connects to a
// configurable WebSocket URL and forwards pre-built JSON objects to the
// event bus.
type WebSocketConnector struct {
	*connector.BaseConnector

	settings Settings
	logger   *slog.Logger
	bus      *eventbus.Bus

	conn     *websocket.Conn
	cancel   context.CancelFunc
	done     chan struct{}
}

// New creates a new WebSocketConnector with the supplied settings, bus, and
// logger. The connector is in the Loaded state and must be started with
// StartAsync.
func New(settings Settings, bus *eventbus.Bus, logger *slog.Logger) *WebSocketConnector {
	if logger == nil {
		logger = slog.Default()
	}

	base := connector.NewBaseConnector(connector.BaseConnectorConfig{
		Name:         "WebSocket",
		Version:      "1.0.0",
		Description:  "Generic WebSocket connector",
		Author:       "VisualHFT",
		ProviderID:   settings.ProviderID,
		ProviderName: settings.ProviderName,
		Bus:          bus,
		Logger:       logger,
	})

	wsc := &WebSocketConnector{
		BaseConnector: base,
		settings:      settings,
		logger:        logger,
		bus:           bus,
	}

	// Register ourselves as the reconnection action so the BaseConnector
	// reconnection loop can call connect on our behalf.
	base.SetReconnectionAction(wsc.connect)

	return wsc
}

// StartAsync connects to the WebSocket server, starts the read loop and
// the health check timer.
func (wsc *WebSocketConnector) StartAsync(ctx context.Context) error {
	if err := wsc.BaseConnector.StartAsync(ctx); err != nil {
		return err
	}

	if err := wsc.connect(ctx); err != nil {
		wsc.SetStatus(enums.PluginStoppedFailed)
		return err
	}

	wsc.SetStatus(enums.PluginStarted)
	wsc.PublishProvider(wsc.GetProviderModel(enums.SessionConnected))

	// Create a cancellable context for the background goroutines.
	readCtx, cancel := context.WithCancel(ctx)
	wsc.cancel = cancel
	wsc.done = make(chan struct{})

	go wsc.readLoop(readCtx)
	go wsc.healthCheck(readCtx)

	return nil
}

// StopAsync gracefully shuts down the connector. It cancels the read loop,
// closes the WebSocket connection, and waits for the read loop to finish.
func (wsc *WebSocketConnector) StopAsync(ctx context.Context) error {
	if wsc.cancel != nil {
		wsc.cancel()
		wsc.cancel = nil
	}

	if wsc.conn != nil {
		_ = wsc.conn.Close()
		wsc.conn = nil
	}

	// Wait for the read loop to exit.
	if wsc.done != nil {
		<-wsc.done
		wsc.done = nil
	}

	return wsc.BaseConnector.StopAsync(ctx)
}

// connect dials the WebSocket server. It is also used as the reconnection
// action by BaseConnector.
func (wsc *WebSocketConnector) connect(ctx context.Context) error {
	url := fmt.Sprintf("ws://%s:%d", wsc.settings.HostName, wsc.settings.Port)

	wsc.logger.Info("websocket connecting", slog.String("url", url))

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}

	wsc.conn = conn
	wsc.logger.Info("websocket connected", slog.String("url", url))
	return nil
}

// readLoop reads JSON messages from the WebSocket connection and dispatches
// them to the event bus. It runs until the context is cancelled or an error
// occurs on the connection.
func (wsc *WebSocketConnector) readLoop(ctx context.Context) {
	defer close(wsc.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if wsc.conn == nil {
			return
		}

		_, msg, err := wsc.conn.ReadMessage()
		if err != nil {
			// If the context was cancelled, this is an expected close.
			select {
			case <-ctx.Done():
				return
			default:
			}
			wsc.logger.Warn("websocket read error", slog.Any("error", err))
			wsc.HandleConnectionLost(ctx, "read error", err)
			return
		}

		wsc.handleMessage(msg)
	}
}

// handleMessage deserializes the envelope and dispatches the data payload
// to the appropriate event bus topic.
func (wsc *WebSocketConnector) handleMessage(raw []byte) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		wsc.logger.Warn("websocket: failed to unmarshal envelope",
			slog.Any("error", err),
		)
		return
	}

	switch env.Type {
	case "Market":
		wsc.handleMarket(env.Data)
	case "Trades":
		wsc.handleTrades(env.Data)
	case "HeartBeats":
		wsc.handleHeartBeats(env.Data)
	default:
		// Unknown message types are silently ignored.
	}
}

// handleMarket deserializes data as []models.OrderBook and publishes each
// order book to the event bus.
func (wsc *WebSocketConnector) handleMarket(data json.RawMessage) {
	var books []models.OrderBook
	if err := json.Unmarshal(data, &books); err != nil {
		wsc.logger.Warn("websocket: failed to unmarshal Market data",
			slog.Any("error", err),
		)
		return
	}
	for i := range books {
		wsc.PublishOrderBook(&books[i])
	}
}

// handleTrades deserializes data as []models.Trade and publishes each trade
// to the event bus.
func (wsc *WebSocketConnector) handleTrades(data json.RawMessage) {
	var trades []models.Trade
	if err := json.Unmarshal(data, &trades); err != nil {
		wsc.logger.Warn("websocket: failed to unmarshal Trades data",
			slog.Any("error", err),
		)
		return
	}
	for _, trade := range trades {
		wsc.PublishTrade(trade)
	}
}

// handleHeartBeats deserializes data as []models.Provider and publishes each
// provider status to the event bus.
func (wsc *WebSocketConnector) handleHeartBeats(data json.RawMessage) {
	var providers []models.Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		wsc.logger.Warn("websocket: failed to unmarshal HeartBeats data",
			slog.Any("error", err),
		)
		return
	}
	for _, provider := range providers {
		wsc.PublishProvider(provider)
	}
}

// healthCheck periodically publishes a provider heartbeat to signal that the
// connector is alive. It runs every 5 seconds until the context is cancelled.
func (wsc *WebSocketConnector) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wsc.PublishProvider(wsc.GetProviderModel(enums.SessionConnected))
		}
	}
}
