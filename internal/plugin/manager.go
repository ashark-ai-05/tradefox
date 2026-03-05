package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ashark-ai-05/tradefox/internal/config"
	"github.com/ashark-ai-05/tradefox/internal/core/enums"
	"github.com/ashark-ai-05/tradefox/internal/core/interfaces"
	"github.com/ashark-ai-05/tradefox/internal/eventbus"
)

// Manager registers, starts, stops, and lists plugins. It is the Go equivalent
// of the C# static PluginManager class. Each running plugin is wrapped in a
// Supervisor that provides panic recovery and automatic restart.
type Manager struct {
	plugins     map[string]interfaces.Plugin  // pluginID -> plugin
	supervisors map[string]*Supervisor        // pluginID -> supervisor (for running plugins)
	cancelFns   map[string]context.CancelFunc // pluginID -> cancel function
	doneChs     map[string]chan struct{}       // pluginID -> done channel (closed when supervisor goroutine exits)
	bus         *eventbus.Bus
	settings    *config.Manager
	mu          sync.RWMutex
	logger      *slog.Logger
}

// NewManager creates a new plugin Manager.
func NewManager(bus *eventbus.Bus, settings *config.Manager, logger *slog.Logger) *Manager {
	return &Manager{
		plugins:     make(map[string]interfaces.Plugin),
		supervisors: make(map[string]*Supervisor),
		cancelFns:   make(map[string]context.CancelFunc),
		doneChs:     make(map[string]chan struct{}),
		bus:         bus,
		settings:    settings,
		logger:      logger,
	}
}

// Register adds a plugin to the manager. Returns an error if the plugin has an
// empty name or a plugin with the same unique ID is already registered.
func (m *Manager) Register(p interfaces.Plugin) error {
	if p.Name() == "" {
		return errors.New("plugin: cannot register plugin with empty name")
	}

	id := p.PluginUniqueID()
	if id == "" {
		return errors.New("plugin: cannot register plugin with empty unique ID")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[id]; exists {
		return fmt.Errorf("plugin: duplicate plugin ID %q", id)
	}

	p.SetStatus(enums.PluginLoaded)
	m.plugins[id] = p

	m.logger.Info("plugin registered",
		"name", p.Name(),
		"id", id,
		"type", p.PluginType(),
	)

	return nil
}

// StartAll starts all registered plugins, each in its own supervised goroutine.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.plugins))
	for id := range m.plugins {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var errs []error
	for _, id := range ids {
		if err := m.StartPlugin(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// StartPlugin starts a single plugin by its unique ID. It creates a child
// context with cancel and wraps the plugin in a Supervisor. The supervisor
// runs in a separate goroutine. For Connectors and Studies, the supervisor
// calls their StartAsync method. Other plugin types are simply marked as
// Started.
func (m *Manager) StartPlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.plugins[id]
	if !ok {
		return fmt.Errorf("plugin: unknown plugin ID %q", id)
	}

	// Guard against double-start.
	status := p.Status()
	if status == enums.PluginStarting || status == enums.PluginStarted {
		return nil
	}

	// Determine the run function based on plugin type.
	var runFn func(ctx context.Context) error

	switch v := p.(type) {
	case interfaces.Connector:
		runFn = func(ctx context.Context) error {
			p.SetStatus(enums.PluginStarting)
			if err := v.StartAsync(ctx); err != nil {
				return err
			}
			p.SetStatus(enums.PluginStarted)
			// Block until context is cancelled.
			<-ctx.Done()
			return nil
		}
	case interfaces.Study:
		runFn = func(ctx context.Context) error {
			p.SetStatus(enums.PluginStarting)
			if err := v.StartAsync(ctx); err != nil {
				return err
			}
			p.SetStatus(enums.PluginStarted)
			// Block until context is cancelled.
			<-ctx.Done()
			return nil
		}
	default:
		// For generic plugins that don't have StartAsync, just mark as started.
		p.SetStatus(enums.PluginStarted)
		return nil
	}

	childCtx, cancel := context.WithCancel(ctx)

	sup := NewSupervisor(p.Name(), runFn, m.logger)
	sup.SetStatusCallback(func(status enums.PluginStatus) {
		p.SetStatus(status)
	})

	m.supervisors[id] = sup
	m.cancelFns[id] = cancel

	done := make(chan struct{})
	m.doneChs[id] = done

	go func() {
		defer close(done)
		if err := sup.Run(childCtx); err != nil {
			m.logger.Error("plugin supervisor exited with error",
				"plugin", p.Name(),
				"id", id,
				"error", err,
			)
			p.SetStatus(enums.PluginStoppedFailed)
		}
	}()

	m.logger.Info("plugin started",
		"name", p.Name(),
		"id", id,
	)

	return nil
}

// StopPlugin stops a single plugin by its unique ID. It cancels the plugin's
// context and, for Connectors and Studies, calls their StopAsync method.
func (m *Manager) StopPlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.plugins[id]
	if !ok {
		return fmt.Errorf("plugin: unknown plugin ID %q", id)
	}

	// Guard against double-stop.
	status := p.Status()
	if status == enums.PluginStopped || status == enums.PluginStoppedFailed {
		return nil
	}

	p.SetStatus(enums.PluginStopping)

	// Cancel the supervised goroutine.
	if cancel, ok := m.cancelFns[id]; ok {
		cancel()
		delete(m.cancelFns, id)
	}

	// Wait for the supervisor goroutine to finish.
	if done, ok := m.doneChs[id]; ok {
		m.mu.Unlock()
		<-done
		m.mu.Lock()
		delete(m.doneChs, id)
	}

	delete(m.supervisors, id)

	// Call StopAsync for Connectors and Studies.
	switch v := p.(type) {
	case interfaces.Connector:
		if err := v.StopAsync(ctx); err != nil {
			p.SetStatus(enums.PluginStoppedFailed)
			return fmt.Errorf("plugin: stop failed for %q: %w", id, err)
		}
	case interfaces.Study:
		if err := v.StopAsync(ctx); err != nil {
			p.SetStatus(enums.PluginStoppedFailed)
			return fmt.Errorf("plugin: stop failed for %q: %w", id, err)
		}
	}

	p.SetStatus(enums.PluginStopped)

	m.logger.Info("plugin stopped",
		"name", p.Name(),
		"id", id,
	)

	return nil
}

// StopAll stops all running plugins.
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	ids := make([]string, 0, len(m.plugins))
	for id := range m.plugins {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	var errs []error
	for _, id := range ids {
		if err := m.StopPlugin(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ListPlugins returns a snapshot of information about all registered plugins.
func (m *Manager) ListPlugins() []interfaces.PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]interfaces.PluginInfo, 0, len(m.plugins))
	for id, p := range m.plugins {
		infos = append(infos, interfaces.PluginInfo{
			ID:          id,
			Name:        p.Name(),
			Version:     p.Version(),
			Description: p.Description(),
			Author:      p.Author(),
			Type:        p.PluginType(),
			Status:      p.Status(),
			License:     p.RequiredLicenseLevel(),
		})
	}

	return infos
}

// GetPlugin returns a plugin by its unique ID. The second return value
// indicates whether the plugin was found.
func (m *Manager) GetPlugin(id string) (interfaces.Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.plugins[id]
	return p, ok
}

// PluginCount returns the number of registered plugins.
func (m *Manager) PluginCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.plugins)
}
