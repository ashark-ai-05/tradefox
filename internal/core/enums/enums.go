// Package enums defines all domain-specific enumeration types used throughout
// VisualHFT. Each enum is a named integer type with String() and JSON
// marshal/unmarshal methods so values serialize as human-readable strings.
package enums

import (
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// OrderSide
// ---------------------------------------------------------------------------

// OrderSide represents the side of an order (Buy/Sell).
type OrderSide int

const (
	OrderSideBuy  OrderSide = iota // 0
	OrderSideSell                  // 1
	OrderSideNone                  // 2
)

var orderSideNames = [...]string{"Buy", "Sell", "None"}

func (s OrderSide) String() string {
	if int(s) >= 0 && int(s) < len(orderSideNames) {
		return orderSideNames[s]
	}
	return "Unknown"
}

func (s OrderSide) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *OrderSide) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range orderSideNames {
		if name == str {
			*s = OrderSide(i)
			return nil
		}
	}
	return fmt.Errorf("invalid OrderSide: %q", str)
}

// ---------------------------------------------------------------------------
// LOBSide (flags/bitmask, byte-sized)
// ---------------------------------------------------------------------------

// LOBSide represents which side of the limit order book. It is a flags enum
// so values can be combined with bitwise OR.
type LOBSide byte

const (
	LOBSideNone LOBSide = 0                    // 0
	LOBSideBid  LOBSide = 1                    // 1
	LOBSideAsk  LOBSide = 2                    // 2
	LOBSideBoth LOBSide = LOBSideBid | LOBSideAsk // 3
)

func (s LOBSide) String() string {
	switch s {
	case LOBSideNone:
		return "None"
	case LOBSideBid:
		return "Bid"
	case LOBSideAsk:
		return "Ask"
	case LOBSideBoth:
		return "Both"
	default:
		return "Unknown"
	}
}

func (s LOBSide) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *LOBSide) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "None":
		*s = LOBSideNone
	case "Bid":
		*s = LOBSideBid
	case "Ask":
		*s = LOBSideAsk
	case "Both":
		*s = LOBSideBoth
	default:
		return fmt.Errorf("invalid LOBSide: %q", str)
	}
	return nil
}

// ---------------------------------------------------------------------------
// OrderStatus
// ---------------------------------------------------------------------------

// OrderStatus represents the lifecycle status of an order.
type OrderStatus int

const (
	OrderStatusNone            OrderStatus = iota // 0
	OrderStatusSent                               // 1
	OrderStatusNew                                // 2
	OrderStatusCanceled                           // 3
	OrderStatusRejected                           // 4
	OrderStatusPartialFilled                      // 5
	OrderStatusFilled                             // 6
	OrderStatusCanceledSent                       // 7
	OrderStatusReplaceSent                        // 8
	OrderStatusReplaced                           // 9
	OrderStatusLastLookHolding                    // 10
)

var orderStatusNames = [...]string{
	"None", "Sent", "New", "Canceled", "Rejected",
	"PartialFilled", "Filled", "CanceledSent",
	"ReplaceSent", "Replaced", "LastLookHolding",
}

func (s OrderStatus) String() string {
	if int(s) >= 0 && int(s) < len(orderStatusNames) {
		return orderStatusNames[s]
	}
	return "Unknown"
}

func (s OrderStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *OrderStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range orderStatusNames {
		if name == str {
			*s = OrderStatus(i)
			return nil
		}
	}
	return fmt.Errorf("invalid OrderStatus: %q", str)
}

// ---------------------------------------------------------------------------
// OrderTimeInForce
// ---------------------------------------------------------------------------

// OrderTimeInForce represents the time-in-force policy of an order.
type OrderTimeInForce int

const (
	TimeInForceNone OrderTimeInForce = iota // 0
	TimeInForceGTC                          // 1
	TimeInForceIOC                          // 2
	TimeInForceFOK                          // 3
	TimeInForceMOK                          // 4
)

var orderTimeInForceNames = [...]string{"None", "GTC", "IOC", "FOK", "MOK"}

func (t OrderTimeInForce) String() string {
	if int(t) >= 0 && int(t) < len(orderTimeInForceNames) {
		return orderTimeInForceNames[t]
	}
	return "Unknown"
}

func (t OrderTimeInForce) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *OrderTimeInForce) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range orderTimeInForceNames {
		if name == str {
			*t = OrderTimeInForce(i)
			return nil
		}
	}
	return fmt.Errorf("invalid OrderTimeInForce: %q", str)
}

// ---------------------------------------------------------------------------
// OrderType
// ---------------------------------------------------------------------------

// OrderType represents the type of an order.
type OrderType int

const (
	OrderTypeLimit  OrderType = iota // 0
	OrderTypeMarket                  // 1
	OrderTypePegged                  // 2
	OrderTypeNone                    // 3
	OrderTypeStopLimit               // 4
)

var orderTypeNames = [...]string{"Limit", "Market", "Pegged", "None", "StopLimit"}

func (t OrderType) String() string {
	if int(t) >= 0 && int(t) < len(orderTypeNames) {
		return orderTypeNames[t]
	}
	return "Unknown"
}

func (t OrderType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *OrderType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range orderTypeNames {
		if name == str {
			*t = OrderType(i)
			return nil
		}
	}
	return fmt.Errorf("invalid OrderType: %q", str)
}

// ---------------------------------------------------------------------------
// SessionStatus
// ---------------------------------------------------------------------------

// SessionStatus represents the connection status of a data provider.
type SessionStatus int

const (
	SessionConnecting            SessionStatus = iota // 0
	SessionConnected                                  // 1
	SessionConnectedWithWarnings                      // 2
	SessionDisconnectedFailed                         // 3
	SessionDisconnected                               // 4
)

var sessionStatusNames = [...]string{
	"Connecting", "Connected", "ConnectedWithWarnings",
	"DisconnectedFailed", "Disconnected",
}

func (s SessionStatus) String() string {
	if int(s) >= 0 && int(s) < len(sessionStatusNames) {
		return sessionStatusNames[s]
	}
	return "Unknown"
}

func (s SessionStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SessionStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range sessionStatusNames {
		if name == str {
			*s = SessionStatus(i)
			return nil
		}
	}
	return fmt.Errorf("invalid SessionStatus: %q", str)
}

// ---------------------------------------------------------------------------
// AggregationLevel
// ---------------------------------------------------------------------------

// AggregationLevel represents the time-based aggregation granularity.
type AggregationLevel int

const (
	AggregationNone  AggregationLevel = iota // 0
	AggregationMs1                           // 1
	AggregationMs10                          // 2
	AggregationMs100                         // 3
	AggregationMs500                         // 4
	AggregationS1                            // 5
	AggregationS3                            // 6
	AggregationS5                            // 7
	AggregationD1                            // 8
)

var aggregationLevelNames = [...]string{
	"None", "Ms1", "Ms10", "Ms100", "Ms500",
	"S1", "S3", "S5", "D1",
}

func (a AggregationLevel) String() string {
	if int(a) >= 0 && int(a) < len(aggregationLevelNames) {
		return aggregationLevelNames[a]
	}
	return "Unknown"
}

// Duration returns the time.Duration that corresponds to this aggregation level.
func (a AggregationLevel) Duration() time.Duration {
	switch a {
	case AggregationNone:
		return 0
	case AggregationMs1:
		return 1 * time.Millisecond
	case AggregationMs10:
		return 10 * time.Millisecond
	case AggregationMs100:
		return 100 * time.Millisecond
	case AggregationMs500:
		return 500 * time.Millisecond
	case AggregationS1:
		return 1 * time.Second
	case AggregationS3:
		return 3 * time.Second
	case AggregationS5:
		return 5 * time.Second
	case AggregationD1:
		return 24 * time.Hour
	default:
		return 0
	}
}

func (a AggregationLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *AggregationLevel) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range aggregationLevelNames {
		if name == str {
			*a = AggregationLevel(i)
			return nil
		}
	}
	return fmt.Errorf("invalid AggregationLevel: %q", str)
}

// ---------------------------------------------------------------------------
// PositionCalcMethod
// ---------------------------------------------------------------------------

// PositionCalcMethod represents the P&L calculation method.
type PositionCalcMethod int

const (
	PositionCalcFIFO PositionCalcMethod = iota // 0
	PositionCalcLIFO                           // 1
)

var positionCalcMethodNames = [...]string{"FIFO", "LIFO"}

func (m PositionCalcMethod) String() string {
	if int(m) >= 0 && int(m) < len(positionCalcMethodNames) {
		return positionCalcMethodNames[m]
	}
	return "Unknown"
}

func (m PositionCalcMethod) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

func (m *PositionCalcMethod) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range positionCalcMethodNames {
		if name == str {
			*m = PositionCalcMethod(i)
			return nil
		}
	}
	return fmt.Errorf("invalid PositionCalcMethod: %q", str)
}

// ---------------------------------------------------------------------------
// MDUpdateAction
// ---------------------------------------------------------------------------

// MDUpdateAction represents the type of market data update.
type MDUpdateAction int

const (
	MDUpdateNew          MDUpdateAction = iota // 0
	MDUpdateChange                             // 1
	MDUpdateDelete                             // 2
	MDUpdateChangeAdjust                       // 3
	MDUpdateReplace                            // 4
	MDUpdateNone                               // 5
)

var mdUpdateActionNames = [...]string{
	"New", "Change", "Delete", "ChangeAdjust", "Replace", "None",
}

func (a MDUpdateAction) String() string {
	if int(a) >= 0 && int(a) < len(mdUpdateActionNames) {
		return mdUpdateActionNames[a]
	}
	return "Unknown"
}

func (a MDUpdateAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *MDUpdateAction) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range mdUpdateActionNames {
		if name == str {
			*a = MDUpdateAction(i)
			return nil
		}
	}
	return fmt.Errorf("invalid MDUpdateAction: %q", str)
}

// ---------------------------------------------------------------------------
// PluginType
// ---------------------------------------------------------------------------

// PluginType represents the type of plugin.
type PluginType int

const (
	PluginTypeUnknown         PluginType = iota // 0
	PluginTypeStudy                             // 1
	PluginTypeMultiStudy                        // 2
	PluginTypeMarketConnector                   // 3
)

var pluginTypeNames = [...]string{"Unknown", "Study", "MultiStudy", "MarketConnector"}

func (t PluginType) String() string {
	if int(t) >= 0 && int(t) < len(pluginTypeNames) {
		return pluginTypeNames[t]
	}
	return "Unknown"
}

func (t PluginType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *PluginType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range pluginTypeNames {
		if name == str {
			*t = PluginType(i)
			return nil
		}
	}
	return fmt.Errorf("invalid PluginType: %q", str)
}

// ---------------------------------------------------------------------------
// LicenseLevel
// ---------------------------------------------------------------------------

// LicenseLevel represents the required license level.
type LicenseLevel int

const (
	LicenseCommunity  LicenseLevel = iota // 0
	LicenseAmateur                        // 1
	LicenseCore                           // 2
	LicensePro                            // 3
	LicenseEnterprise                     // 4
)

var licenseLevelNames = [...]string{"Community", "Amateur", "Core", "Pro", "Enterprise"}

func (l LicenseLevel) String() string {
	if int(l) >= 0 && int(l) < len(licenseLevelNames) {
		return licenseLevelNames[l]
	}
	return "Unknown"
}

func (l LicenseLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

func (l *LicenseLevel) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range licenseLevelNames {
		if name == str {
			*l = LicenseLevel(i)
			return nil
		}
	}
	return fmt.Errorf("invalid LicenseLevel: %q", str)
}

// ---------------------------------------------------------------------------
// PluginStatus
// ---------------------------------------------------------------------------

// PluginStatus represents the lifecycle status of a plugin.
// Matches C# enum ordering: Loading → Loaded → Starting → Started → ...
type PluginStatus int

const (
	PluginLoading        PluginStatus = iota // 0
	PluginLoaded                             // 1
	PluginStarting                           // 2
	PluginStarted                            // 3
	PluginStopping                           // 4
	PluginStopped                            // 5
	PluginStoppedFailed                      // 6
	PluginMalfunctioning                     // 7
)

var pluginStatusNames = [...]string{
	"Loading", "Loaded", "Starting", "Started", "Stopping",
	"Stopped", "StoppedFailed", "Malfunctioning",
}

func (s PluginStatus) String() string {
	if int(s) >= 0 && int(s) < len(pluginStatusNames) {
		return pluginStatusNames[s]
	}
	return "Unknown"
}

func (s PluginStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *PluginStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range pluginStatusNames {
		if name == str {
			*s = PluginStatus(i)
			return nil
		}
	}
	return fmt.Errorf("invalid PluginStatus: %q", str)
}

// ---------------------------------------------------------------------------
// ConditionOperator
// ---------------------------------------------------------------------------

// ConditionOperator represents trigger condition comparison operators.
type ConditionOperator int

const (
	ConditionEquals       ConditionOperator = iota // 0
	ConditionGreaterThan                           // 1
	ConditionLessThan                              // 2
	ConditionCrossesAbove                          // 3
	ConditionCrossesBelow                          // 4
)

var conditionOperatorNames = [...]string{
	"Equals", "GreaterThan", "LessThan", "CrossesAbove", "CrossesBelow",
}

func (o ConditionOperator) String() string {
	if int(o) >= 0 && int(o) < len(conditionOperatorNames) {
		return conditionOperatorNames[o]
	}
	return "Unknown"
}

func (o ConditionOperator) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

func (o *ConditionOperator) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range conditionOperatorNames {
		if name == str {
			*o = ConditionOperator(i)
			return nil
		}
	}
	return fmt.Errorf("invalid ConditionOperator: %q", str)
}

// ---------------------------------------------------------------------------
// ActionType
// ---------------------------------------------------------------------------

// ActionType represents trigger action types.
type ActionType int

const (
	ActionUIAlert ActionType = iota // 0
	ActionRestAPI                   // 1
)

var actionTypeNames = [...]string{"UIAlert", "RestAPI"}

func (a ActionType) String() string {
	if int(a) >= 0 && int(a) < len(actionTypeNames) {
		return actionTypeNames[a]
	}
	return "Unknown"
}

func (a ActionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

func (a *ActionType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range actionTypeNames {
		if name == str {
			*a = ActionType(i)
			return nil
		}
	}
	return fmt.Errorf("invalid ActionType: %q", str)
}

// ---------------------------------------------------------------------------
// TimeWindowUnit
// ---------------------------------------------------------------------------

// TimeWindowUnit represents the time unit for cooldown periods.
type TimeWindowUnit int

const (
	TimeWindowSeconds TimeWindowUnit = iota // 0
	TimeWindowMinutes                       // 1
	TimeWindowHours                         // 2
	TimeWindowDays                          // 3
)

var timeWindowUnitNames = [...]string{"Seconds", "Minutes", "Hours", "Days"}

func (u TimeWindowUnit) String() string {
	if int(u) >= 0 && int(u) < len(timeWindowUnitNames) {
		return timeWindowUnitNames[u]
	}
	return "Unknown"
}

// Duration returns the base time.Duration that one unit represents.
func (u TimeWindowUnit) Duration() time.Duration {
	switch u {
	case TimeWindowSeconds:
		return time.Second
	case TimeWindowMinutes:
		return time.Minute
	case TimeWindowHours:
		return time.Hour
	case TimeWindowDays:
		return 24 * time.Hour
	default:
		return 0
	}
}

func (u TimeWindowUnit) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

func (u *TimeWindowUnit) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for i, name := range timeWindowUnitNames {
		if name == str {
			*u = TimeWindowUnit(i)
			return nil
		}
	}
	return fmt.Errorf("invalid TimeWindowUnit: %q", str)
}
