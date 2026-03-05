package trigger

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

func testEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "triggers.json")
	return NewEngine(configPath, slog.Default()), configPath
}

func TestEngine_AddAndListRules(t *testing.T) {
	e, _ := testEngine(t)

	rule := TriggerRule{
		RuleID:    1,
		Name:      "test-rule",
		IsEnabled: true,
		Conditions: []TriggerCondition{
			{ConditionID: 1, Plugin: "VPIN", Metric: "value", Operator: enums.ConditionGreaterThan, Threshold: 0.8},
		},
		Actions: []TriggerAction{
			{ActionID: 1, Type: enums.ActionUIAlert},
		},
	}

	if err := e.AddOrUpdateRule(rule); err != nil {
		t.Fatalf("AddOrUpdateRule failed: %v", err)
	}

	rules := e.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Name != "test-rule" {
		t.Errorf("expected name test-rule, got %s", rules[0].Name)
	}
}

func TestEngine_UpdateExistingRule(t *testing.T) {
	e, _ := testEngine(t)

	rule := TriggerRule{RuleID: 1, Name: "original", IsEnabled: true}
	_ = e.AddOrUpdateRule(rule)

	rule.Name = "updated"
	_ = e.AddOrUpdateRule(rule)

	rules := e.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after update, got %d", len(rules))
	}
	if rules[0].Name != "updated" {
		t.Errorf("expected name updated, got %s", rules[0].Name)
	}
}

func TestEngine_RemoveRule(t *testing.T) {
	e, _ := testEngine(t)

	_ = e.AddOrUpdateRule(TriggerRule{RuleID: 1, Name: "r1"})
	_ = e.AddOrUpdateRule(TriggerRule{RuleID: 2, Name: "r2"})

	if err := e.RemoveRule(1); err != nil {
		t.Fatalf("RemoveRule failed: %v", err)
	}

	rules := e.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after remove, got %d", len(rules))
	}
	if rules[0].Name != "r2" {
		t.Errorf("expected remaining rule r2, got %s", rules[0].Name)
	}
}

func TestEngine_RemoveNonExistent(t *testing.T) {
	e, _ := testEngine(t)

	err := e.RemoveRule(999)
	if err == nil {
		t.Error("expected error removing non-existent rule")
	}
}

func TestEngine_PersistAndLoad(t *testing.T) {
	e, configPath := testEngine(t)

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID: 1, Name: "persisted", IsEnabled: true,
		Conditions: []TriggerCondition{
			{ConditionID: 1, Plugin: "LOB", Metric: "imbalance", Operator: enums.ConditionLessThan, Threshold: -0.5},
		},
	})

	// Create a new engine pointing to the same config.
	e2 := NewEngine(configPath, slog.Default())
	if err := e2.LoadRules(); err != nil {
		t.Fatalf("LoadRules failed: %v", err)
	}

	rules := e2.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule from loaded config, got %d", len(rules))
	}
	if rules[0].Name != "persisted" {
		t.Errorf("expected name persisted, got %s", rules[0].Name)
	}
}

func TestEngine_LoadNonExistent(t *testing.T) {
	e := NewEngine("/tmp/nonexistent-trigger-config.json", slog.Default())
	if err := e.LoadRules(); err != nil {
		t.Errorf("LoadRules should not error for non-existent file, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Condition evaluation
// ---------------------------------------------------------------------------

func TestCondition_GreaterThan(t *testing.T) {
	cond := TriggerCondition{Operator: enums.ConditionGreaterThan, Threshold: 0.5}
	if !evaluateCondition(cond, 0.6, 0, false) {
		t.Error("0.6 > 0.5 should be true")
	}
	if evaluateCondition(cond, 0.4, 0, false) {
		t.Error("0.4 > 0.5 should be false")
	}
}

func TestCondition_LessThan(t *testing.T) {
	cond := TriggerCondition{Operator: enums.ConditionLessThan, Threshold: 0.5}
	if !evaluateCondition(cond, 0.3, 0, false) {
		t.Error("0.3 < 0.5 should be true")
	}
	if evaluateCondition(cond, 0.7, 0, false) {
		t.Error("0.7 < 0.5 should be false")
	}
}

func TestCondition_Equals(t *testing.T) {
	cond := TriggerCondition{Operator: enums.ConditionEquals, Threshold: 1.0}
	if !evaluateCondition(cond, 1.0, 0, false) {
		t.Error("1.0 == 1.0 should be true")
	}
	if evaluateCondition(cond, 1.1, 0, false) {
		t.Error("1.1 == 1.0 should be false")
	}
}

func TestCondition_CrossesAbove(t *testing.T) {
	cond := TriggerCondition{Operator: enums.ConditionCrossesAbove, Threshold: 0.5}

	// Previous below, current above → true.
	if !evaluateCondition(cond, 0.6, 0.4, true) {
		t.Error("0.4 → 0.6 should cross above 0.5")
	}

	// Previous already above → false.
	if evaluateCondition(cond, 0.7, 0.6, true) {
		t.Error("0.6 → 0.7 should not cross above (was already above)")
	}

	// No previous value → false.
	if evaluateCondition(cond, 0.6, 0, false) {
		t.Error("no previous value should not cross above")
	}
}

func TestCondition_CrossesBelow(t *testing.T) {
	cond := TriggerCondition{Operator: enums.ConditionCrossesBelow, Threshold: 0.5}

	// Previous above, current below → true.
	if !evaluateCondition(cond, 0.4, 0.6, true) {
		t.Error("0.6 → 0.4 should cross below 0.5")
	}

	// Previous already below → false.
	if evaluateCondition(cond, 0.3, 0.4, true) {
		t.Error("0.4 → 0.3 should not cross below (was already below)")
	}
}

// ---------------------------------------------------------------------------
// Full processing with cooldown
// ---------------------------------------------------------------------------

func TestEngine_ProcessMetric_FiresAction(t *testing.T) {
	e, _ := testEngine(t)

	var fired sync.WaitGroup
	fired.Add(1)
	var firedRule string
	e.ActionCallback = func(rule TriggerRule, action TriggerAction, event MetricEvent) {
		firedRule = rule.Name
		fired.Done()
	}

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID:    1,
		Name:      "alert-rule",
		IsEnabled: true,
		Conditions: []TriggerCondition{
			{Plugin: "VPIN", Metric: "value", Operator: enums.ConditionGreaterThan, Threshold: 0.8},
		},
		Actions: []TriggerAction{
			{ActionID: 1, Type: enums.ActionUIAlert},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = e.Start(ctx)

	e.RegisterMetric(MetricEvent{Plugin: "VPIN", Metric: "value", Value: 0.9, Timestamp: time.Now()})

	fired.Wait()
	if firedRule != "alert-rule" {
		t.Errorf("expected rule alert-rule, got %s", firedRule)
	}
}

func TestEngine_Cooldown(t *testing.T) {
	e, _ := testEngine(t)

	fireCount := 0
	var mu sync.Mutex
	e.ActionCallback = func(rule TriggerRule, action TriggerAction, event MetricEvent) {
		mu.Lock()
		fireCount++
		mu.Unlock()
	}

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID:    1,
		Name:      "cooldown-rule",
		IsEnabled: true,
		Conditions: []TriggerCondition{
			{Plugin: "test", Metric: "val", Operator: enums.ConditionGreaterThan, Threshold: 0},
		},
		Actions: []TriggerAction{
			{ActionID: 1, Type: enums.ActionUIAlert, CooldownDuration: 10, CooldownUnit: enums.TimeWindowSeconds},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = e.Start(ctx)

	// Fire twice rapidly.
	e.RegisterMetric(MetricEvent{Plugin: "test", Metric: "val", Value: 1.0, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond) // Let first process.
	e.RegisterMetric(MetricEvent{Plugin: "test", Metric: "val", Value: 2.0, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond) // Let second process.

	mu.Lock()
	count := fireCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 fire (second should be in cooldown), got %d", count)
	}
}

func TestEngine_DisabledRule(t *testing.T) {
	e, _ := testEngine(t)

	fired := false
	e.ActionCallback = func(rule TriggerRule, action TriggerAction, event MetricEvent) {
		fired = true
	}

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID:    1,
		Name:      "disabled",
		IsEnabled: false,
		Conditions: []TriggerCondition{
			{Plugin: "test", Metric: "val", Operator: enums.ConditionGreaterThan, Threshold: 0},
		},
		Actions: []TriggerAction{
			{ActionID: 1, Type: enums.ActionUIAlert},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = e.Start(ctx)

	e.RegisterMetric(MetricEvent{Plugin: "test", Metric: "val", Value: 1.0, Timestamp: time.Now()})
	time.Sleep(50 * time.Millisecond)

	if fired {
		t.Error("disabled rule should not fire")
	}
}

// ---------------------------------------------------------------------------
// REST API action
// ---------------------------------------------------------------------------

func TestEngine_RestAPIAction(t *testing.T) {
	var receivedBody string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		mu.Lock()
		receivedBody = string(body)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	e, _ := testEngine(t)
	e.httpClient = srv.Client()

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID:    1,
		Name:      "webhook-rule",
		IsEnabled: true,
		Conditions: []TriggerCondition{
			{Plugin: "test", Metric: "val", Operator: enums.ConditionGreaterThan, Threshold: 0},
		},
		Actions: []TriggerAction{
			{
				ActionID: 1,
				Type:     enums.ActionRestAPI,
				RestAPI: &RestAPIAction{
					URL:          srv.URL,
					Method:       "POST",
					BodyTemplate: `{"rule":"{{rulename}}","value":"{{value}}"}`,
					Headers:      map[string]string{"X-Custom": "test"},
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = e.Start(ctx)

	e.RegisterMetric(MetricEvent{Plugin: "test", Metric: "val", Value: 42.0, Timestamp: time.Now()})
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := receivedBody
	mu.Unlock()

	if body == "" {
		t.Fatal("expected webhook to receive body")
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed["rule"] != "webhook-rule" {
		t.Errorf("expected rule=webhook-rule, got %s", parsed["rule"])
	}
}

// ---------------------------------------------------------------------------
// CooldownSpan
// ---------------------------------------------------------------------------

func TestCooldownSpan(t *testing.T) {
	tests := []struct {
		dur  int
		unit enums.TimeWindowUnit
		want time.Duration
	}{
		{5, enums.TimeWindowSeconds, 5 * time.Second},
		{2, enums.TimeWindowMinutes, 2 * time.Minute},
		{1, enums.TimeWindowHours, 1 * time.Hour},
		{3, enums.TimeWindowDays, 3 * 24 * time.Hour},
	}

	for _, tt := range tests {
		a := &TriggerAction{CooldownDuration: tt.dur, CooldownUnit: tt.unit}
		if got := a.CooldownSpan(); got != tt.want {
			t.Errorf("CooldownSpan(%d, %s) = %v, want %v", tt.dur, tt.unit, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// MetricEvent key
// ---------------------------------------------------------------------------

func TestMetricEvent_Key(t *testing.T) {
	e := MetricEvent{Plugin: "VPIN", Metric: "value", Exchange: "binance", Symbol: "BTCUSD"}
	expected := "VPIN.value.binance.BTCUSD"
	if got := e.metricKey(); got != expected {
		t.Errorf("expected key %s, got %s", expected, got)
	}
}

// ---------------------------------------------------------------------------
// Config persistence format
// ---------------------------------------------------------------------------

func TestEngine_ConfigFormat(t *testing.T) {
	e, configPath := testEngine(t)

	_ = e.AddOrUpdateRule(TriggerRule{
		RuleID:    42,
		Name:      "format-test",
		IsEnabled: true,
		Conditions: []TriggerCondition{
			{ConditionID: 1, Plugin: "VPIN", Metric: "value", Operator: enums.ConditionCrossesAbove, Threshold: 0.9},
		},
		Actions: []TriggerAction{
			{
				ActionID:         1,
				Type:             enums.ActionRestAPI,
				CooldownDuration: 60,
				CooldownUnit:     enums.TimeWindowSeconds,
				RestAPI: &RestAPIAction{
					URL:          "https://example.com/webhook",
					Method:       "POST",
					BodyTemplate: `{"alert":"{{rulename}}"}`,
				},
			},
		},
	})

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var rules []TriggerRule
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	if len(rules) != 1 || rules[0].RuleID != 42 {
		t.Errorf("unexpected config content: %s", string(data))
	}
}
