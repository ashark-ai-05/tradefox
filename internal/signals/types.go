package signals

// SignalSet holds the computed values for all signals at a point in time.
type SignalSet struct {
	Microprice MicropriceSignal `json:"microprice"`
	OFI        OFISignal        `json:"ofi"`
	DepthImb   DepthImbSignal   `json:"depthImb"`
	Sweep      SweepSignal      `json:"sweep"`
	Lambda     LambdaSignal     `json:"lambda"`
	Vol        VolSignal        `json:"vol"`
	Spoof      SpoofSignal      `json:"spoof"`
	Composite  CompositeSignal  `json:"composite"`
}

type MicropriceSignal struct {
	Value  float64 `json:"value"`
	Mid    float64 `json:"mid"`
	DivBps float64 `json:"divBps"`
	Dir    string  `json:"dir"`
}

type OFISignal struct {
	Value float64 `json:"value"`
	Dir   string  `json:"dir"`
}

type DepthImbSignal struct {
	Levels   []float64 `json:"levels"`
	Weighted float64   `json:"weighted"`
	Pressure string    `json:"pressure"`
}

type SweepSignal struct {
	Active bool    `json:"active"`
	Dir    string  `json:"dir,omitempty"`
	Levels int     `json:"levels"`
	Size   float64 `json:"size"`
}

type LambdaSignal struct {
	Value  float64 `json:"value"`
	Regime string  `json:"regime"`
}

type VolSignal struct {
	Realized float64 `json:"realized"`
	Regime   string  `json:"regime"`
	Trend    string  `json:"trend"`
}

type SpoofSignal struct {
	Score  float64 `json:"score"`
	Active bool    `json:"active"`
	Side   string  `json:"side,omitempty"`
}

type CompositeSignal struct {
	Avg      float64 `json:"avg"`
	Dir      string  `json:"dir"`
	Strength float64 `json:"strength"`
}
