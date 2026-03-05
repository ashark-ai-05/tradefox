package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// Engine evaluates trigger rules against incoming metrics and fires actions.
type Engine struct {
	rules           []TriggerRule
	mu              sync.RWMutex
	lastMetricValues sync.Map // metricKey → float64
	actionLastFired  sync.Map // "ruleID.actionID" → time.Time
	metricCh        chan MetricEvent
	configPath      string
	logger          *slog.Logger
	httpClient      *http.Client

	// ActionCallback is invoked for UI alert actions (testable).
	ActionCallback func(rule TriggerRule, action TriggerAction, event MetricEvent)
}

// NewEngine creates a new trigger engine with the given config path and logger.
// If configPath is empty, defaults to ~/.visualhft/triggers.json.
func NewEngine(configPath string, logger *slog.Logger) *Engine {
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".visualhft", "triggers.json")
	}
	return &Engine{
		configPath: configPath,
		metricCh:   make(chan MetricEvent, 10000),
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Start begins the background metric evaluation worker.
func (e *Engine) Start(ctx context.Context) error {
	if err := e.LoadRules(); err != nil {
		e.logger.Warn("trigger engine: failed to load rules", slog.Any("error", err))
	}

	go e.processLoop(ctx)
	e.logger.Info("trigger engine started", slog.Int("rules", len(e.rules)))
	return nil
}

// RegisterMetric enqueues a metric event for evaluation. Non-blocking.
func (e *Engine) RegisterMetric(event MetricEvent) {
	select {
	case e.metricCh <- event:
	default:
		e.logger.Warn("trigger engine: metric dropped, channel full")
	}
}

// ---------------------------------------------------------------------------
// Rule management
// ---------------------------------------------------------------------------

// AddOrUpdateRule adds a new rule or updates an existing one. Saves to disk.
func (e *Engine) AddOrUpdateRule(rule TriggerRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	updated := false
	for i, r := range e.rules {
		if r.RuleID == rule.RuleID {
			e.rules[i] = rule
			updated = true
			break
		}
	}
	if !updated {
		e.rules = append(e.rules, rule)
	}

	return e.saveRulesLocked()
}

// RemoveRule removes a rule by ID. Saves to disk.
func (e *Engine) RemoveRule(ruleID int64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, r := range e.rules {
		if r.RuleID == ruleID {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return e.saveRulesLocked()
		}
	}
	return fmt.Errorf("rule %d not found", ruleID)
}

// ListRules returns a copy of all rules.
func (e *Engine) ListRules() []TriggerRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]TriggerRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// LoadRules reads rules from the JSON config file.
func (e *Engine) LoadRules() error {
	data, err := os.ReadFile(e.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no config file is fine
		}
		return fmt.Errorf("read trigger config: %w", err)
	}

	var rules []TriggerRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("parse trigger config: %w", err)
	}

	e.mu.Lock()
	e.rules = rules
	e.mu.Unlock()
	return nil
}

// SaveRules writes rules to the JSON config file.
func (e *Engine) SaveRules() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.saveRulesLocked()
}

// saveRulesLocked writes rules to disk. Caller must hold e.mu.
func (e *Engine) saveRulesLocked() error {
	dir := filepath.Dir(e.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create trigger config dir: %w", err)
	}

	data, err := json.MarshalIndent(e.rules, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trigger rules: %w", err)
	}

	return os.WriteFile(e.configPath, data, 0o644)
}

// ---------------------------------------------------------------------------
// Background worker
// ---------------------------------------------------------------------------

func (e *Engine) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-e.metricCh:
			e.processMetric(event)
		}
	}
}

func (e *Engine) processMetric(event MetricEvent) {
	key := event.metricKey()

	// Retrieve previous value for CrossesAbove/CrossesBelow.
	var previousValue float64
	var hasPrevious bool
	if prev, ok := e.lastMetricValues.Load(key); ok {
		previousValue = prev.(float64)
		hasPrevious = true
	}

	// Store current value.
	e.lastMetricValues.Store(key, event.Value)

	e.mu.RLock()
	rules := make([]TriggerRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, rule := range rules {
		if !rule.IsEnabled {
			continue
		}

		if e.evaluateConditions(rule.Conditions, event, previousValue, hasPrevious) {
			e.fireActions(rule, event)
		}
	}
}

// evaluateConditions checks if all conditions for a rule are satisfied.
func (e *Engine) evaluateConditions(conditions []TriggerCondition, event MetricEvent, previousValue float64, hasPrevious bool) bool {
	for _, cond := range conditions {
		// Only evaluate conditions that match the metric's plugin.
		if cond.Plugin != "" && cond.Plugin != event.Plugin {
			continue
		}
		if cond.Metric != "" && cond.Metric != event.Metric {
			continue
		}

		if !evaluateCondition(cond, event.Value, previousValue, hasPrevious) {
			return false
		}
	}
	return true
}

// evaluateCondition checks a single condition against the current value.
func evaluateCondition(cond TriggerCondition, currentValue, previousValue float64, hasPrevious bool) bool {
	switch cond.Operator {
	case enums.ConditionEquals:
		return currentValue == cond.Threshold
	case enums.ConditionGreaterThan:
		return currentValue > cond.Threshold
	case enums.ConditionLessThan:
		return currentValue < cond.Threshold
	case enums.ConditionCrossesAbove:
		return hasPrevious && previousValue < cond.Threshold && currentValue >= cond.Threshold
	case enums.ConditionCrossesBelow:
		return hasPrevious && previousValue > cond.Threshold && currentValue <= cond.Threshold
	default:
		return false
	}
}

// fireActions executes all actions for a triggered rule, subject to cooldown.
func (e *Engine) fireActions(rule TriggerRule, event MetricEvent) {
	for _, action := range rule.Actions {
		cooldownKey := fmt.Sprintf("%d.%d", rule.RuleID, action.ActionID)

		// Check cooldown.
		if lastFired, ok := e.actionLastFired.Load(cooldownKey); ok {
			elapsed := time.Since(lastFired.(time.Time))
			if elapsed < action.CooldownSpan() {
				continue // still in cooldown
			}
		}

		// Record fire time.
		e.actionLastFired.Store(cooldownKey, time.Now())

		// Execute action.
		switch action.Type {
		case enums.ActionRestAPI:
			go e.executeRestAPI(rule, action, event)
		case enums.ActionUIAlert:
			if e.ActionCallback != nil {
				e.ActionCallback(rule, action, event)
			}
		}
	}
}

// executeRestAPI sends the REST API webhook.
func (e *Engine) executeRestAPI(rule TriggerRule, action TriggerAction, event MetricEvent) {
	if action.RestAPI == nil {
		return
	}

	body := e.expandTemplate(action.RestAPI.BodyTemplate, rule, action, event)

	method := strings.ToUpper(action.RestAPI.Method)
	if method == "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, action.RestAPI.URL, bytes.NewBufferString(body))
	if err != nil {
		e.logger.Error("trigger: create HTTP request failed",
			slog.String("rule", rule.Name),
			slog.Any("error", err),
		)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range action.RestAPI.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.logger.Error("trigger: HTTP request failed",
			slog.String("rule", rule.Name),
			slog.String("url", action.RestAPI.URL),
			slog.Any("error", err),
		)
		return
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		e.logger.Warn("trigger: HTTP request returned error status",
			slog.String("rule", rule.Name),
			slog.Int("status", resp.StatusCode),
		)
	}
}

// expandTemplate replaces {{placeholders}} in the body template.
func (e *Engine) expandTemplate(template string, rule TriggerRule, action TriggerAction, event MetricEvent) string {
	r := strings.NewReplacer(
		"{{rulename}}", rule.Name,
		"{{plugin}}", event.Plugin,
		"{{metric}}", event.Metric,
		"{{exchange}}", event.Exchange,
		"{{symbol}}", event.Symbol,
		"{{value}}", fmt.Sprintf("%f", event.Value),
		"{{timestamp}}", event.Timestamp.Format(time.RFC3339),
	)

	// Add condition info if available.
	condStr := ""
	thresholdStr := ""
	if len(rule.Conditions) > 0 {
		condStr = rule.Conditions[0].Operator.String()
		thresholdStr = fmt.Sprintf("%f", rule.Conditions[0].Threshold)
	}

	r2 := strings.NewReplacer(
		"{{condition}}", condStr,
		"{{threshold}}", thresholdStr,
	)

	return r2.Replace(r.Replace(template))
}
