package plugin

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockPlugin satisfies interfaces.Plugin for testing.
type mockPlugin struct {
	mu          sync.Mutex
	name        string
	version     string
	description string
	author      string
	pluginType  enums.PluginType
	uniqueID    string
	license     enums.LicenseLevel
	status      enums.PluginStatus
}

func (p *mockPlugin) Name() string                          { return p.name }
func (p *mockPlugin) Version() string                       { return p.version }
func (p *mockPlugin) Description() string                   { return p.description }
func (p *mockPlugin) Author() string                        { return p.author }
func (p *mockPlugin) PluginType() enums.PluginType          { return p.pluginType }
func (p *mockPlugin) PluginUniqueID() string                { return p.uniqueID }
func (p *mockPlugin) RequiredLicenseLevel() enums.LicenseLevel { return p.license }

func (p *mockPlugin) Status() enums.PluginStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *mockPlugin) SetStatus(s enums.PluginStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

// mockConnector satisfies both interfaces.Plugin and interfaces.Connector.
type mockConnector struct {
	mockPlugin
	startFn func(ctx context.Context) error
	stopFn  func(ctx context.Context) error
}

func (c *mockConnector) StartAsync(ctx context.Context) error {
	if c.startFn != nil {
		return c.startFn(ctx)
	}
	return nil
}

func (c *mockConnector) StopAsync(ctx context.Context) error {
	if c.stopFn != nil {
		return c.stopFn(ctx)
	}
	return nil
}

// Compile-time interface checks.
var _ interfaces.Plugin = (*mockPlugin)(nil)
var _ interfaces.Connector = (*mockConnector)(nil)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	logger := newTestLogger()
	bus := eventbus.NewBus(logger)
	t.Cleanup(bus.Close)
	settings := config.NewManager(t.TempDir() + "/settings.json")
	return NewManager(bus, settings, logger)
}

func newMockConnector(id, name string) *mockConnector {
	return &mockConnector{
		mockPlugin: mockPlugin{
			name:        name,
			version:     "1.0.0",
			description: "test connector",
			author:      "test",
			pluginType:  enums.PluginTypeMarketConnector,
			uniqueID:    id,
			license:     enums.LicenseCommunity,
		},
	}
}

func newMockPluginOnly(id, name string) *mockPlugin {
	return &mockPlugin{
		name:        name,
		version:     "0.1.0",
		description: "test plugin",
		author:      "test",
		pluginType:  enums.PluginTypeStudy,
		uniqueID:    id,
		license:     enums.LicenseCommunity,
	}
}

// waitForStatus polls until the plugin reaches the expected status or timeout.
func waitForStatus(p interfaces.Plugin, expected enums.PluginStatus, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p.Status() == expected {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestManager_Register(t *testing.T) {
	m := newTestManager(t)
	p := newMockConnector("conn-1", "TestConnector")

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() returned unexpected error: %v", err)
	}

	if m.PluginCount() != 1 {
		t.Fatalf("expected 1 plugin, got %d", m.PluginCount())
	}

	infos := m.ListPlugins()
	if len(infos) != 1 {
		t.Fatalf("expected 1 PluginInfo, got %d", len(infos))
	}
	if infos[0].Name != "TestConnector" {
		t.Errorf("expected name 'TestConnector', got %q", infos[0].Name)
	}
	if infos[0].Status != enums.PluginLoaded {
		t.Errorf("expected status PluginLoaded, got %v", infos[0].Status)
	}
}

func TestManager_Register_Duplicate(t *testing.T) {
	m := newTestManager(t)
	p1 := newMockConnector("conn-1", "Connector1")
	p2 := newMockConnector("conn-1", "Connector1Again")

	if err := m.Register(p1); err != nil {
		t.Fatalf("first Register() failed: %v", err)
	}

	err := m.Register(p2)
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
}

func TestManager_Register_EmptyName(t *testing.T) {
	m := newTestManager(t)
	p := newMockConnector("conn-1", "")

	err := m.Register(p)
	if err == nil {
		t.Fatal("expected error on empty name, got nil")
	}
}

func TestManager_Register_EmptyID(t *testing.T) {
	m := newTestManager(t)
	p := newMockConnector("", "SomeName")

	err := m.Register(p)
	if err == nil {
		t.Fatal("expected error on empty unique ID, got nil")
	}
}

func TestManager_StartPlugin(t *testing.T) {
	m := newTestManager(t)

	started := make(chan struct{})
	p := newMockConnector("conn-1", "TestConnector")
	p.startFn = func(ctx context.Context) error {
		close(started)
		return nil
	}

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	ctx := context.Background()
	if err := m.StartPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StartPlugin() failed: %v", err)
	}

	// Wait for the connector's StartAsync to have been called.
	select {
	case <-started:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartAsync to be called")
	}

	// The plugin should transition to Started.
	if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
		t.Errorf("expected status Started, got %v", p.Status())
	}

	// Cleanup.
	if err := m.StopPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StopPlugin() failed: %v", err)
	}
}

func TestManager_StopPlugin(t *testing.T) {
	m := newTestManager(t)

	p := newMockConnector("conn-1", "TestConnector")
	stopCalled := false
	p.stopFn = func(ctx context.Context) error {
		stopCalled = true
		return nil
	}

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	ctx := context.Background()
	if err := m.StartPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StartPlugin() failed: %v", err)
	}

	// Wait for Started.
	if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
		t.Fatalf("plugin did not reach Started status, got %v", p.Status())
	}

	if err := m.StopPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StopPlugin() failed: %v", err)
	}

	if p.Status() != enums.PluginStopped {
		t.Errorf("expected status Stopped, got %v", p.Status())
	}

	if !stopCalled {
		t.Error("expected StopAsync to have been called")
	}
}

func TestManager_StartAll(t *testing.T) {
	m := newTestManager(t)

	plugins := make([]*mockConnector, 3)
	for i := 0; i < 3; i++ {
		id := "conn-" + string(rune('a'+i))
		name := "Connector-" + string(rune('A'+i))
		p := newMockConnector(id, name)
		plugins[i] = p
		if err := m.Register(p); err != nil {
			t.Fatalf("Register() failed for %s: %v", id, err)
		}
	}

	ctx := context.Background()
	if err := m.StartAll(ctx); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}

	for _, p := range plugins {
		if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
			t.Errorf("plugin %s did not reach Started, got %v", p.Name(), p.Status())
		}
	}

	// Cleanup.
	if err := m.StopAll(ctx); err != nil {
		t.Fatalf("StopAll() failed: %v", err)
	}
}

func TestManager_StopAll(t *testing.T) {
	m := newTestManager(t)

	plugins := make([]*mockConnector, 3)
	for i := 0; i < 3; i++ {
		id := "conn-" + string(rune('a'+i))
		name := "Connector-" + string(rune('A'+i))
		p := newMockConnector(id, name)
		plugins[i] = p
		if err := m.Register(p); err != nil {
			t.Fatalf("Register() failed for %s: %v", id, err)
		}
	}

	ctx := context.Background()
	if err := m.StartAll(ctx); err != nil {
		t.Fatalf("StartAll() failed: %v", err)
	}

	// Wait for all to be started.
	for _, p := range plugins {
		if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
			t.Fatalf("plugin %s did not reach Started, got %v", p.Name(), p.Status())
		}
	}

	if err := m.StopAll(ctx); err != nil {
		t.Fatalf("StopAll() failed: %v", err)
	}

	for _, p := range plugins {
		if p.Status() != enums.PluginStopped {
			t.Errorf("plugin %s expected Stopped, got %v", p.Name(), p.Status())
		}
	}
}

func TestManager_ListPlugins(t *testing.T) {
	m := newTestManager(t)

	p := newMockConnector("conn-1", "TestConnector")
	p.version = "2.3.4"
	p.description = "A test connector"
	p.author = "TestAuthor"
	p.pluginType = enums.PluginTypeMarketConnector
	p.license = enums.LicensePro

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	infos := m.ListPlugins()
	if len(infos) != 1 {
		t.Fatalf("expected 1 PluginInfo, got %d", len(infos))
	}

	info := infos[0]
	if info.ID != "conn-1" {
		t.Errorf("expected ID 'conn-1', got %q", info.ID)
	}
	if info.Name != "TestConnector" {
		t.Errorf("expected Name 'TestConnector', got %q", info.Name)
	}
	if info.Version != "2.3.4" {
		t.Errorf("expected Version '2.3.4', got %q", info.Version)
	}
	if info.Description != "A test connector" {
		t.Errorf("expected Description 'A test connector', got %q", info.Description)
	}
	if info.Author != "TestAuthor" {
		t.Errorf("expected Author 'TestAuthor', got %q", info.Author)
	}
	if info.Type != enums.PluginTypeMarketConnector {
		t.Errorf("expected Type MarketConnector, got %v", info.Type)
	}
	if info.Status != enums.PluginLoaded {
		t.Errorf("expected Status Loaded, got %v", info.Status)
	}
	if info.License != enums.LicensePro {
		t.Errorf("expected License Pro, got %v", info.License)
	}
}

func TestManager_GetPlugin(t *testing.T) {
	m := newTestManager(t)
	p := newMockConnector("conn-1", "TestConnector")

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Found case.
	got, ok := m.GetPlugin("conn-1")
	if !ok {
		t.Fatal("GetPlugin() returned false for registered plugin")
	}
	if got.Name() != "TestConnector" {
		t.Errorf("expected name 'TestConnector', got %q", got.Name())
	}

	// Not-found case.
	_, ok = m.GetPlugin("nonexistent")
	if ok {
		t.Fatal("GetPlugin() returned true for nonexistent plugin")
	}
}

func TestManager_StartPlugin_Idempotent(t *testing.T) {
	m := newTestManager(t)

	startCount := 0
	var mu sync.Mutex
	p := newMockConnector("conn-1", "TestConnector")
	p.startFn = func(ctx context.Context) error {
		mu.Lock()
		startCount++
		mu.Unlock()
		return nil
	}

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	ctx := context.Background()
	if err := m.StartPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("first StartPlugin() failed: %v", err)
	}

	// Wait for the plugin to reach Started.
	if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
		t.Fatalf("plugin did not reach Started, got %v", p.Status())
	}

	// Second start should be a no-op (no error).
	if err := m.StartPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("second StartPlugin() returned error: %v", err)
	}

	// Give a moment to ensure no extra goroutine was started.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	count := startCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected StartAsync called once, got %d", count)
	}

	// Cleanup.
	if err := m.StopPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StopPlugin() failed: %v", err)
	}
}

func TestManager_StopPlugin_Idempotent(t *testing.T) {
	m := newTestManager(t)

	stopCount := 0
	var mu sync.Mutex
	p := newMockConnector("conn-1", "TestConnector")
	p.stopFn = func(ctx context.Context) error {
		mu.Lock()
		stopCount++
		mu.Unlock()
		return nil
	}

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	ctx := context.Background()
	if err := m.StartPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("StartPlugin() failed: %v", err)
	}

	if !waitForStatus(p, enums.PluginStarted, 2*time.Second) {
		t.Fatalf("plugin did not reach Started, got %v", p.Status())
	}

	// First stop.
	if err := m.StopPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("first StopPlugin() failed: %v", err)
	}

	if p.Status() != enums.PluginStopped {
		t.Fatalf("expected Stopped, got %v", p.Status())
	}

	// Second stop should be a no-op (no error).
	if err := m.StopPlugin(ctx, "conn-1"); err != nil {
		t.Fatalf("second StopPlugin() returned error: %v", err)
	}

	mu.Lock()
	count := stopCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected StopAsync called once, got %d", count)
	}
}

func TestManager_StartPlugin_UnknownID(t *testing.T) {
	m := newTestManager(t)

	err := m.StartPlugin(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin ID, got nil")
	}
}

func TestManager_StopPlugin_UnknownID(t *testing.T) {
	m := newTestManager(t)

	err := m.StopPlugin(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown plugin ID, got nil")
	}
}

func TestManager_GenericPlugin_StartSetsStatus(t *testing.T) {
	// A generic plugin (not Connector or Study) should be set to Started
	// directly without creating a supervisor goroutine.
	m := newTestManager(t)
	p := newMockPluginOnly("generic-1", "GenericPlugin")

	if err := m.Register(p); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	ctx := context.Background()
	if err := m.StartPlugin(ctx, "generic-1"); err != nil {
		t.Fatalf("StartPlugin() failed: %v", err)
	}

	if p.Status() != enums.PluginStarted {
		t.Errorf("expected status Started, got %v", p.Status())
	}
}
