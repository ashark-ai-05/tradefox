// Package trigger implements a rule-based trigger engine that evaluates
// metric conditions and fires actions (REST API webhooks, UI alerts).
package trigger

import (
	"time"

	"github.com/ashark-ai-05/tradefox/internal/core/enums"
)

// TriggerRule defines a complete trigger workflow: a set of conditions
// that must all be satisfied, and a set of actions to execute when they are.
type TriggerRule struct {
	RuleID     int64              `json:"ruleId"`
	Name       string             `json:"name"`
	Conditions []TriggerCondition `json:"conditions"`
	Actions    []TriggerAction    `json:"actions"`
	IsEnabled  bool               `json:"isEnabled"`
}

// TriggerCondition is a single predicate on a metric value.
type TriggerCondition struct {
	ConditionID int64                    `json:"conditionId"`
	Plugin      string                   `json:"plugin"`
	Metric      string                   `json:"metric"`
	Operator    enums.ConditionOperator   `json:"operator"`
	Threshold   float64                  `json:"threshold"`
}

// TriggerAction describes what to do when a rule fires.
type TriggerAction struct {
	ActionID         int64                `json:"actionId"`
	Type             enums.ActionType     `json:"type"`
	RestAPI          *RestAPIAction       `json:"restApi,omitempty"`
	UIAlert          *UIAlertAction       `json:"uiAlert,omitempty"`
	CooldownDuration int                  `json:"cooldownDuration"`
	CooldownUnit     enums.TimeWindowUnit `json:"cooldownUnit"`
}

// CooldownSpan returns the cooldown as a time.Duration.
func (a *TriggerAction) CooldownSpan() time.Duration {
	return time.Duration(a.CooldownDuration) * a.CooldownUnit.Duration()
}

// RestAPIAction configures a REST API webhook call.
type RestAPIAction struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	BodyTemplate string            `json:"bodyTemplate"`
	Headers      map[string]string `json:"headers"`
}

// UIAlertAction configures an in-app notification.
type UIAlertAction struct {
	Message  string `json:"message"`
	Severity string `json:"severity"` // "info", "warning", "error"
}

// MetricEvent represents a single metric data point registered by a plugin.
type MetricEvent struct {
	Plugin    string
	Metric    string
	Exchange  string
	Symbol    string
	Value     float64
	Timestamp time.Time
}

// metricKey returns the composite key for tracking metric values.
func (m MetricEvent) metricKey() string {
	return m.Plugin + "." + m.Metric + "." + m.Exchange + "." + m.Symbol
}
