// Package connector provides the BaseConnector foundation for all exchange
// connectors in VisualHFT. It implements the interfaces.Connector (and
// therefore interfaces.Plugin) contract, providing lifecycle management,
// reconnection with exponential backoff, symbol normalization, decimal-place
// detection, and data dispatch via the event bus.
package connector

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/models"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

const maxReconnectAttempts = 5

// BaseConnector provides the common foundation for all exchange connectors.
// Derived connectors embed BaseConnector and override StartAsync / StopAsync
// as needed.
type BaseConnector struct {
	name        string
	version     string
	description string
	author      string
	status      atomic.Value // stores enums.PluginStatus
	pluginUniqueID string

	providerID   int
	providerName string

	bus      *eventbus.Bus
	settings *config.Manager
	logger   *slog.Logger

	symbolMap map[string]string
	symbolMu  sync.RWMutex

	reconnectFn          func(ctx context.Context) error
	reconnectSem         chan struct{}
	isReconnecting       atomic.Int32
	reconnectAttempt     int
	reconnectMu          sync.Mutex
	maxReconnectAttempts int
}

// BaseConnectorConfig holds the parameters required to initialise a
// BaseConnector.
type BaseConnectorConfig struct {
	Name         string
	Version      string
	Description  string
	Author       string
	ProviderID   int
	ProviderName string
	Bus          *eventbus.Bus
	Settings     *config.Manager
	Logger       *slog.Logger
}

// NewBaseConnector creates a BaseConnector initialised from the supplied
// configuration. The PluginUniqueID is computed once during construction.
func NewBaseConnector(cfg BaseConnectorConfig) *BaseConnector {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	bc := &BaseConnector{
		name:                 cfg.Name,
		version:              cfg.Version,
		description:          cfg.Description,
		author:               cfg.Author,
		providerID:           cfg.ProviderID,
		providerName:         cfg.ProviderName,
		bus:                  cfg.Bus,
		settings:             cfg.Settings,
		logger:               cfg.Logger,
		symbolMap:            make(map[string]string),
		reconnectSem:         make(chan struct{}, 1),
		maxReconnectAttempts: maxReconnectAttempts,
	}

	// Compute the plugin unique ID (SHA256 of Name+Author+Version+Description).
	bc.pluginUniqueID = computePluginID(bc.name, bc.author, bc.version, bc.description)

	// Initial status.
	bc.status.Store(enums.PluginLoaded)

	return bc
}

// computePluginID returns the hex-encoded SHA256 hash of the concatenated
// plugin metadata fields.
func computePluginID(name, author, version, description string) string {
	h := sha256.New()
	h.Write([]byte(name + author + version + description))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ---------------------------------------------------------------------------
// Plugin interface
// ---------------------------------------------------------------------------

func (bc *BaseConnector) Name() string                          { return bc.name }
func (bc *BaseConnector) Version() string                       { return bc.version }
func (bc *BaseConnector) Description() string                   { return bc.description }
func (bc *BaseConnector) Author() string                        { return bc.author }
func (bc *BaseConnector) PluginType() enums.PluginType          { return enums.PluginTypeMarketConnector }
func (bc *BaseConnector) PluginUniqueID() string                { return bc.pluginUniqueID }
func (bc *BaseConnector) RequiredLicenseLevel() enums.LicenseLevel { return enums.LicenseCommunity }

// Status returns the current plugin status.
func (bc *BaseConnector) Status() enums.PluginStatus {
	v := bc.status.Load()
	if v == nil {
		return enums.PluginLoaded
	}
	return v.(enums.PluginStatus)
}

// SetStatus atomically stores the new plugin status.
func (bc *BaseConnector) SetStatus(s enums.PluginStatus) {
	bc.status.Store(s)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// StartAsync sets the connector status to Starting, resets reconnection
// counters, and publishes a "Connecting" provider status to the event bus.
func (bc *BaseConnector) StartAsync(ctx context.Context) error {
	bc.SetStatus(enums.PluginStarting)

	bc.reconnectMu.Lock()
	bc.reconnectAttempt = 0
	bc.reconnectMu.Unlock()
	bc.isReconnecting.Store(0)

	bc.PublishProvider(bc.GetProviderModel(enums.SessionConnecting))
	return nil
}

// StopAsync sets the connector status to Stopped.
func (bc *BaseConnector) StopAsync(ctx context.Context) error {
	bc.SetStatus(enums.PluginStopped)
	return nil
}

// ---------------------------------------------------------------------------
// Reconnection with exponential backoff
// ---------------------------------------------------------------------------

// SetReconnectionAction registers the function that will be called on each
// reconnection attempt. Typically this is the derived connector's connect
// method.
func (bc *BaseConnector) SetReconnectionAction(fn func(ctx context.Context) error) {
	bc.reconnectMu.Lock()
	defer bc.reconnectMu.Unlock()
	bc.reconnectFn = fn
}

// HandleConnectionLost is the entry point for triggering reconnection logic.
// It uses an atomic flag to ensure only one goroutine runs the reconnection
// loop at a time.
func (bc *BaseConnector) HandleConnectionLost(ctx context.Context, reason string, err error) {
	// Ensure only one goroutine enters the reconnection logic.
	if !bc.isReconnecting.CompareAndSwap(0, 1) {
		bc.logger.Info("reconnection already in progress, skipping",
			slog.String("reason", reason),
		)
		return
	}

	// Acquire the semaphore to serialize execution.
	bc.reconnectSem <- struct{}{}

	go func() {
		defer func() {
			<-bc.reconnectSem
			bc.isReconnecting.Store(0)
		}()

		bc.logger.Warn("connection lost, starting reconnection",
			slog.String("reason", reason),
			slog.Any("error", err),
		)

		bc.reconnectMu.Lock()
		bc.reconnectAttempt = 0
		reconnectFn := bc.reconnectFn
		bc.reconnectMu.Unlock()

		if reconnectFn == nil {
			bc.logger.Error("no reconnection action set, cannot reconnect")
			bc.SetStatus(enums.PluginStoppedFailed)
			bc.PublishProvider(bc.GetProviderModel(enums.SessionDisconnectedFailed))
			return
		}

		for attempt := 1; attempt <= bc.maxReconnectAttempts; attempt++ {
			select {
			case <-ctx.Done():
				bc.logger.Info("reconnection cancelled by context")
				return
			default:
			}

			bc.reconnectMu.Lock()
			bc.reconnectAttempt = attempt
			bc.reconnectMu.Unlock()

			// Exponential backoff: 2^attempt seconds + random jitter 0-999ms.
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			delay := backoff + jitter

			bc.logger.Info("reconnection attempt",
				slog.Int("attempt", attempt),
				slog.Int("maxAttempts", bc.maxReconnectAttempts),
				slog.Duration("backoff", delay),
			)

			select {
			case <-ctx.Done():
				bc.logger.Info("reconnection cancelled by context during backoff")
				return
			case <-time.After(delay):
			}

			// Stop the current connection.
			if stopErr := bc.StopAsync(ctx); stopErr != nil {
				bc.logger.Warn("error during stop before reconnect",
					slog.Any("error", stopErr),
				)
			}

			// Attempt reconnection.
			if reconnErr := reconnectFn(ctx); reconnErr != nil {
				bc.logger.Warn("reconnection attempt failed",
					slog.Int("attempt", attempt),
					slog.Any("error", reconnErr),
				)
				continue
			}

			// Restart.
			if startErr := bc.StartAsync(ctx); startErr != nil {
				bc.logger.Warn("start after reconnect failed",
					slog.Int("attempt", attempt),
					slog.Any("error", startErr),
				)
				continue
			}

			// Success.
			bc.logger.Info("reconnection successful", slog.Int("attempt", attempt))
			bc.PublishProvider(bc.GetProviderModel(enums.SessionConnected))
			return
		}

		// Exhausted all attempts.
		bc.logger.Error("reconnection failed after max attempts",
			slog.Int("maxAttempts", bc.maxReconnectAttempts),
		)
		bc.SetStatus(enums.PluginStoppedFailed)
		bc.PublishProvider(bc.GetProviderModel(enums.SessionDisconnectedFailed))
	}()
}

// ---------------------------------------------------------------------------
// Symbol normalization
// ---------------------------------------------------------------------------

// ParseSymbols parses a symbol configuration string of the form:
//
//	"BTCUSDT(BTC/USDT), ETHUSDT, XRPUSDT(XRP/USD)"
//
// Each entry is separated by commas. If an entry contains '(' the part before
// it is the exchange (raw) symbol and the part inside the parentheses (without
// the trailing ')') is the normalized name. If there are no parentheses the
// exchange symbol is used as both key and normalized value.
func (bc *BaseConnector) ParseSymbols(input string) {
	bc.symbolMu.Lock()
	defer bc.symbolMu.Unlock()

	bc.symbolMap = make(map[string]string)

	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if idx := strings.Index(part, "("); idx >= 0 {
			exchangeSym := strings.TrimSpace(part[:idx])
			rest := part[idx+1:]
			rest = strings.TrimSuffix(rest, ")")
			normalized := strings.TrimSpace(rest)
			bc.symbolMap[exchangeSym] = normalized
		} else {
			bc.symbolMap[part] = part
		}
	}
}

// GetNormalizedSymbol returns the normalized symbol for the given exchange
// symbol. If the exchange symbol is not in the map, the original symbol is
// returned unchanged.
func (bc *BaseConnector) GetNormalizedSymbol(sym string) string {
	bc.symbolMu.RLock()
	defer bc.symbolMu.RUnlock()

	if normalized, ok := bc.symbolMap[sym]; ok {
		return normalized
	}
	return sym
}

// GetAllExchangeSymbols returns all raw exchange symbols from the symbol map.
func (bc *BaseConnector) GetAllExchangeSymbols() []string {
	bc.symbolMu.RLock()
	defer bc.symbolMu.RUnlock()

	symbols := make([]string, 0, len(bc.symbolMap))
	for k := range bc.symbolMap {
		symbols = append(symbols, k)
	}
	return symbols
}

// GetAllNormalizedSymbols returns all normalized symbols from the symbol map.
func (bc *BaseConnector) GetAllNormalizedSymbols() []string {
	bc.symbolMu.RLock()
	defer bc.symbolMu.RUnlock()

	symbols := make([]string, 0, len(bc.symbolMap))
	for _, v := range bc.symbolMap {
		symbols = append(symbols, v)
	}
	return symbols
}

// ---------------------------------------------------------------------------
// Decimal place detection
// ---------------------------------------------------------------------------

// RecognizeDecimalPlaces finds the maximum number of decimal places across the
// supplied float64 values. The minimum return value is 1.
func RecognizeDecimalPlaces(values []float64) int {
	maxPlaces := 0
	for _, v := range values {
		s := fmt.Sprintf("%g", v)
		if idx := strings.Index(s, "."); idx >= 0 {
			places := len(s) - idx - 1
			if places > maxPlaces {
				maxPlaces = places
			}
		}
	}
	if maxPlaces < 1 {
		return 1
	}
	return maxPlaces
}

// ---------------------------------------------------------------------------
// Data dispatch (publish to event bus)
// ---------------------------------------------------------------------------

// PublishOrderBook publishes an order book snapshot to the event bus.
func (bc *BaseConnector) PublishOrderBook(ob *models.OrderBook) {
	if bc.bus != nil {
		bc.bus.OrderBooks.Publish(ob)
	}
}

// PublishTrade publishes a trade to the event bus.
func (bc *BaseConnector) PublishTrade(trade models.Trade) {
	if bc.bus != nil {
		bc.bus.Trades.Publish(trade)
	}
}

// PublishProvider publishes a provider status update to the event bus.
func (bc *BaseConnector) PublishProvider(provider models.Provider) {
	if bc.bus != nil {
		bc.bus.Providers.Publish(provider)
	}
}

// PublishOrder publishes an order to the positions topic on the event bus.
func (bc *BaseConnector) PublishOrder(order models.Order) {
	if bc.bus != nil {
		bc.bus.Positions.Publish(order)
	}
}

// GetProviderModel creates a Provider model from the connector's settings
// with the given session status.
func (bc *BaseConnector) GetProviderModel(status enums.SessionStatus) models.Provider {
	return models.Provider{
		ProviderID:   bc.providerID,
		ProviderCode: bc.providerID,
		ProviderName: bc.providerName,
		Status:       status,
		LastUpdated:  time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Settings persistence
// ---------------------------------------------------------------------------

// SaveToUserSettings serializes the given settings and stores them under this
// connector's plugin unique ID.
func (bc *BaseConnector) SaveToUserSettings(settings any) error {
	if bc.settings == nil {
		return fmt.Errorf("connector: settings manager is nil")
	}
	if err := bc.settings.SetPluginSettings(bc.pluginUniqueID, settings); err != nil {
		return fmt.Errorf("connector: saving plugin settings: %w", err)
	}
	return bc.settings.Save()
}

// LoadFromUserSettings deserializes the stored settings for this connector's
// plugin unique ID into the target.
func (bc *BaseConnector) LoadFromUserSettings(target any) error {
	if bc.settings == nil {
		return fmt.Errorf("connector: settings manager is nil")
	}
	return bc.settings.GetPluginSettings(bc.pluginUniqueID, target)
}
