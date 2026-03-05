package enums

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// String() tests — at least 3 values per enum type
// ---------------------------------------------------------------------------

func TestOrderSideString(t *testing.T) {
	tests := []struct {
		side OrderSide
		want string
	}{
		{OrderSideBuy, "Buy"},
		{OrderSideSell, "Sell"},
		{OrderSideNone, "None"},
	}
	for _, tt := range tests {
		if got := tt.side.String(); got != tt.want {
			t.Errorf("OrderSide(%d).String() = %q, want %q", tt.side, got, tt.want)
		}
	}
}

func TestLOBSideString(t *testing.T) {
	tests := []struct {
		side LOBSide
		want string
	}{
		{LOBSideNone, "None"},
		{LOBSideBid, "Bid"},
		{LOBSideAsk, "Ask"},
		{LOBSideBoth, "Both"},
	}
	for _, tt := range tests {
		if got := tt.side.String(); got != tt.want {
			t.Errorf("LOBSide(%d).String() = %q, want %q", tt.side, got, tt.want)
		}
	}
}

func TestOrderStatusString(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   string
	}{
		{OrderStatusNone, "None"},
		{OrderStatusNew, "New"},
		{OrderStatusFilled, "Filled"},
		{OrderStatusLastLookHolding, "LastLookHolding"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("OrderStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestOrderTimeInForceString(t *testing.T) {
	tests := []struct {
		tif  OrderTimeInForce
		want string
	}{
		{TimeInForceNone, "None"},
		{TimeInForceGTC, "GTC"},
		{TimeInForceIOC, "IOC"},
		{TimeInForceFOK, "FOK"},
		{TimeInForceMOK, "MOK"},
	}
	for _, tt := range tests {
		if got := tt.tif.String(); got != tt.want {
			t.Errorf("OrderTimeInForce(%d).String() = %q, want %q", tt.tif, got, tt.want)
		}
	}
}

func TestOrderTypeString(t *testing.T) {
	tests := []struct {
		ot   OrderType
		want string
	}{
		{OrderTypeLimit, "Limit"},
		{OrderTypeMarket, "Market"},
		{OrderTypePegged, "Pegged"},
		{OrderTypeNone, "None"},
	}
	for _, tt := range tests {
		if got := tt.ot.String(); got != tt.want {
			t.Errorf("OrderType(%d).String() = %q, want %q", tt.ot, got, tt.want)
		}
	}
}

func TestSessionStatusString(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{SessionConnecting, "Connecting"},
		{SessionConnected, "Connected"},
		{SessionConnectedWithWarnings, "ConnectedWithWarnings"},
		{SessionDisconnectedFailed, "DisconnectedFailed"},
		{SessionDisconnected, "Disconnected"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("SessionStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestAggregationLevelString(t *testing.T) {
	tests := []struct {
		level AggregationLevel
		want  string
	}{
		{AggregationNone, "None"},
		{AggregationMs1, "Ms1"},
		{AggregationMs100, "Ms100"},
		{AggregationS1, "S1"},
		{AggregationD1, "D1"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("AggregationLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestPositionCalcMethodString(t *testing.T) {
	tests := []struct {
		method PositionCalcMethod
		want   string
	}{
		{PositionCalcFIFO, "FIFO"},
		{PositionCalcLIFO, "LIFO"},
		{PositionCalcMethod(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.method.String(); got != tt.want {
			t.Errorf("PositionCalcMethod(%d).String() = %q, want %q", tt.method, got, tt.want)
		}
	}
}

func TestMDUpdateActionString(t *testing.T) {
	tests := []struct {
		action MDUpdateAction
		want   string
	}{
		{MDUpdateNew, "New"},
		{MDUpdateChange, "Change"},
		{MDUpdateDelete, "Delete"},
		{MDUpdateChangeAdjust, "ChangeAdjust"},
		{MDUpdateReplace, "Replace"},
		{MDUpdateNone, "None"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("MDUpdateAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestPluginTypeString(t *testing.T) {
	tests := []struct {
		pt   PluginType
		want string
	}{
		{PluginTypeUnknown, "Unknown"},
		{PluginTypeStudy, "Study"},
		{PluginTypeMultiStudy, "MultiStudy"},
		{PluginTypeMarketConnector, "MarketConnector"},
	}
	for _, tt := range tests {
		if got := tt.pt.String(); got != tt.want {
			t.Errorf("PluginType(%d).String() = %q, want %q", tt.pt, got, tt.want)
		}
	}
}

func TestLicenseLevelString(t *testing.T) {
	tests := []struct {
		ll   LicenseLevel
		want string
	}{
		{LicenseCommunity, "Community"},
		{LicenseAmateur, "Amateur"},
		{LicensePro, "Pro"},
		{LicenseEnterprise, "Enterprise"},
	}
	for _, tt := range tests {
		if got := tt.ll.String(); got != tt.want {
			t.Errorf("LicenseLevel(%d).String() = %q, want %q", tt.ll, got, tt.want)
		}
	}
}

func TestPluginStatusString(t *testing.T) {
	tests := []struct {
		status PluginStatus
		want   string
	}{
		{PluginLoaded, "Loaded"},
		{PluginStarting, "Starting"},
		{PluginStarted, "Started"},
		{PluginStopping, "Stopping"},
		{PluginStopped, "Stopped"},
		{PluginStoppedFailed, "StoppedFailed"},
		{PluginMalfunctioning, "Malfunctioning"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("PluginStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestConditionOperatorString(t *testing.T) {
	tests := []struct {
		op   ConditionOperator
		want string
	}{
		{ConditionEquals, "Equals"},
		{ConditionGreaterThan, "GreaterThan"},
		{ConditionLessThan, "LessThan"},
		{ConditionCrossesAbove, "CrossesAbove"},
		{ConditionCrossesBelow, "CrossesBelow"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("ConditionOperator(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

func TestActionTypeString(t *testing.T) {
	tests := []struct {
		at   ActionType
		want string
	}{
		{ActionUIAlert, "UIAlert"},
		{ActionRestAPI, "RestAPI"},
		{ActionType(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.at.String(); got != tt.want {
			t.Errorf("ActionType(%d).String() = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestTimeWindowUnitString(t *testing.T) {
	tests := []struct {
		u    TimeWindowUnit
		want string
	}{
		{TimeWindowSeconds, "Seconds"},
		{TimeWindowMinutes, "Minutes"},
		{TimeWindowHours, "Hours"},
		{TimeWindowDays, "Days"},
	}
	for _, tt := range tests {
		if got := tt.u.String(); got != tt.want {
			t.Errorf("TimeWindowUnit(%d).String() = %q, want %q", tt.u, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// LOBSide bitmask test
// ---------------------------------------------------------------------------

func TestLOBSideFlags(t *testing.T) {
	both := LOBSideBid | LOBSideAsk
	if both != LOBSideBoth {
		t.Errorf("LOBSideBid | LOBSideAsk = %d, want %d (LOBSideBoth)", both, LOBSideBoth)
	}
	if LOBSideBid&LOBSideBoth == 0 {
		t.Error("LOBSideBoth should contain LOBSideBid")
	}
	if LOBSideAsk&LOBSideBoth == 0 {
		t.Error("LOBSideBoth should contain LOBSideAsk")
	}
	if LOBSideNone != 0 {
		t.Errorf("LOBSideNone = %d, want 0", LOBSideNone)
	}
}

// ---------------------------------------------------------------------------
// JSON marshal/unmarshal round-trip tests
// ---------------------------------------------------------------------------

func TestOrderSideJSON(t *testing.T) {
	for _, side := range []OrderSide{OrderSideBuy, OrderSideSell, OrderSideNone} {
		data, err := json.Marshal(side)
		if err != nil {
			t.Fatalf("Marshal OrderSide(%d): %v", side, err)
		}
		var got OrderSide
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal OrderSide from %s: %v", data, err)
		}
		if got != side {
			t.Errorf("JSON round-trip: got %d, want %d", got, side)
		}
	}
}

func TestSessionStatusJSON(t *testing.T) {
	for _, s := range []SessionStatus{
		SessionConnecting, SessionConnected, SessionConnectedWithWarnings,
		SessionDisconnectedFailed, SessionDisconnected,
	} {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal SessionStatus(%d): %v", s, err)
		}
		var got SessionStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal SessionStatus from %s: %v", data, err)
		}
		if got != s {
			t.Errorf("JSON round-trip: got %d, want %d", got, s)
		}
	}
}

func TestPluginStatusJSON(t *testing.T) {
	for _, s := range []PluginStatus{
		PluginLoaded, PluginStarting, PluginStarted,
		PluginStopping, PluginStopped, PluginStoppedFailed, PluginMalfunctioning,
	} {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal PluginStatus(%d): %v", s, err)
		}
		var got PluginStatus
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal PluginStatus from %s: %v", data, err)
		}
		if got != s {
			t.Errorf("JSON round-trip: got %d, want %d", got, s)
		}
	}
}

func TestOrderSideJSONSerializesAsString(t *testing.T) {
	data, err := json.Marshal(OrderSideBuy)
	if err != nil {
		t.Fatal(err)
	}
	want := `"Buy"`
	if string(data) != want {
		t.Errorf("OrderSideBuy JSON = %s, want %s", data, want)
	}
}

func TestSessionStatusJSONSerializesAsString(t *testing.T) {
	data, err := json.Marshal(SessionConnectedWithWarnings)
	if err != nil {
		t.Fatal(err)
	}
	want := `"ConnectedWithWarnings"`
	if string(data) != want {
		t.Errorf("SessionConnectedWithWarnings JSON = %s, want %s", data, want)
	}
}

func TestPluginStatusJSONSerializesAsString(t *testing.T) {
	data, err := json.Marshal(PluginMalfunctioning)
	if err != nil {
		t.Fatal(err)
	}
	want := `"Malfunctioning"`
	if string(data) != want {
		t.Errorf("PluginMalfunctioning JSON = %s, want %s", data, want)
	}
}

func TestJSONUnmarshalInvalidValue(t *testing.T) {
	var side OrderSide
	err := json.Unmarshal([]byte(`"InvalidValue"`), &side)
	if err == nil {
		t.Error("expected error for invalid OrderSide JSON, got nil")
	}
}

// ---------------------------------------------------------------------------
// JSON round-trip for struct embedding
// ---------------------------------------------------------------------------

func TestJSONStructRoundTrip(t *testing.T) {
	type order struct {
		Side   OrderSide   `json:"side"`
		Type   OrderType   `json:"type"`
		Status OrderStatus `json:"status"`
	}
	orig := order{
		Side:   OrderSideBuy,
		Type:   OrderTypeLimit,
		Status: OrderStatusFilled,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Verify it contains string values
	want := `{"side":"Buy","type":"Limit","status":"Filled"}`
	if string(data) != want {
		t.Errorf("JSON = %s, want %s", data, want)
	}
	var got order
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip: got %+v, want %+v", got, orig)
	}
}

// ---------------------------------------------------------------------------
// AggregationLevel.Duration() tests
// ---------------------------------------------------------------------------

func TestAggregationLevelDuration(t *testing.T) {
	tests := []struct {
		level AggregationLevel
		want  time.Duration
	}{
		{AggregationNone, 0},
		{AggregationMs1, 1 * time.Millisecond},
		{AggregationMs10, 10 * time.Millisecond},
		{AggregationMs100, 100 * time.Millisecond},
		{AggregationMs500, 500 * time.Millisecond},
		{AggregationS1, 1 * time.Second},
		{AggregationS3, 3 * time.Second},
		{AggregationS5, 5 * time.Second},
		{AggregationD1, 24 * time.Hour},
	}
	for _, tt := range tests {
		if got := tt.level.Duration(); got != tt.want {
			t.Errorf("AggregationLevel(%d).Duration() = %v, want %v", tt.level, got, tt.want)
		}
	}
}

func TestAggregationLevelDurationOutOfRange(t *testing.T) {
	if got := AggregationLevel(99).Duration(); got != 0 {
		t.Errorf("AggregationLevel(99).Duration() = %v, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// TimeWindowUnit.Duration() tests
// ---------------------------------------------------------------------------

func TestTimeWindowUnitDuration(t *testing.T) {
	tests := []struct {
		unit TimeWindowUnit
		want time.Duration
	}{
		{TimeWindowSeconds, time.Second},
		{TimeWindowMinutes, time.Minute},
		{TimeWindowHours, time.Hour},
		{TimeWindowDays, 24 * time.Hour},
	}
	for _, tt := range tests {
		if got := tt.unit.Duration(); got != tt.want {
			t.Errorf("TimeWindowUnit(%d).Duration() = %v, want %v", tt.unit, got, tt.want)
		}
	}
}

func TestTimeWindowUnitDurationOutOfRange(t *testing.T) {
	if got := TimeWindowUnit(99).Duration(); got != 0 {
		t.Errorf("TimeWindowUnit(99).Duration() = %v, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Boundary / out-of-range tests
// ---------------------------------------------------------------------------

func TestOutOfRangeReturnsUnknown(t *testing.T) {
	tests := []struct {
		name string
		got  string
	}{
		{"OrderSide(99)", OrderSide(99).String()},
		{"LOBSide(99)", LOBSide(99).String()},
		{"OrderStatus(99)", OrderStatus(99).String()},
		{"OrderTimeInForce(99)", OrderTimeInForce(99).String()},
		{"OrderType(99)", OrderType(99).String()},
		{"SessionStatus(99)", SessionStatus(99).String()},
		{"AggregationLevel(99)", AggregationLevel(99).String()},
		{"PositionCalcMethod(99)", PositionCalcMethod(99).String()},
		{"MDUpdateAction(99)", MDUpdateAction(99).String()},
		{"PluginType(99)", PluginType(99).String()},
		{"LicenseLevel(99)", LicenseLevel(99).String()},
		{"PluginStatus(99)", PluginStatus(99).String()},
		{"ConditionOperator(99)", ConditionOperator(99).String()},
		{"ActionType(99)", ActionType(99).String()},
		{"TimeWindowUnit(99)", TimeWindowUnit(99).String()},
	}
	for _, tt := range tests {
		if tt.got != "Unknown" {
			t.Errorf("%s.String() = %q, want %q", tt.name, tt.got, "Unknown")
		}
	}
}

func TestNegativeValueReturnsUnknown(t *testing.T) {
	if got := OrderSide(-1).String(); got != "Unknown" {
		t.Errorf("OrderSide(-1).String() = %q, want %q", got, "Unknown")
	}
	if got := OrderStatus(-5).String(); got != "Unknown" {
		t.Errorf("OrderStatus(-5).String() = %q, want %q", got, "Unknown")
	}
}
