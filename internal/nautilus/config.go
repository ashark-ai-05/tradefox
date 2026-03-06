package nautilus

// NautilusConfig holds configuration for the NautilusTrader bridge.
type NautilusConfig struct {
	Enabled       bool       `json:"enabled"`
	PythonPath    string     `json:"python_path"`
	GRPCPort      int        `json:"grpc_port"`
	GRPCAddress   string     `json:"grpc_address"`
	AutoStart     bool       `json:"auto_start"`
	DataCatalog   string     `json:"data_catalog"`
	StrategiesDir string     `json:"strategies_dir"`
	Risk          RiskConfig `json:"risk"`
}

// RiskConfig holds risk management limits.
type RiskConfig struct {
	MaxPositionSizePct float64 `json:"max_position_size_pct"`
	DailyLossLimitPct  float64 `json:"daily_loss_limit_pct"`
	WeeklyLossLimitPct float64 `json:"weekly_loss_limit_pct"`
	MaxDrawdownPct     float64 `json:"max_drawdown_pct"`
	KillSwitchEnabled  bool    `json:"kill_switch_enabled"`
}

// DefaultConfig returns a NautilusConfig with sensible defaults.
func DefaultConfig() NautilusConfig {
	return NautilusConfig{
		Enabled:       false,
		PythonPath:    "python3",
		GRPCPort:      50051,
		GRPCAddress:   "localhost",
		AutoStart:     true,
		DataCatalog:   "~/.tradefox/data",
		StrategiesDir: "~/.tradefox/strategies",
		Risk: RiskConfig{
			MaxPositionSizePct: 2.0,
			DailyLossLimitPct:  3.0,
			WeeklyLossLimitPct: 5.0,
			MaxDrawdownPct:     10.0,
			KillSwitchEnabled:  true,
		},
	}
}
