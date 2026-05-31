package perspectives

type SourceType uint8

const (
	SourceNone SourceType = iota
	SourceFluid
	SourceHawkes
	SourcePumpDump
	SourceDepthFlow
	SourceSentiment
	SourceCorrelation
	SourceCausal
	SourceLeadLag
	SourceLiquidity
	SourceExhaustion
	SourcePrediction
	SourceCVD
	SourceToxicity
)

// sourceNames maps each source to the canonical lower-case name the dashboard
// gauges key on.
var sourceNames = map[SourceType]string{
	SourceFluid:       "fluid",
	SourceHawkes:      "hawkes",
	SourcePumpDump:    "pumpdump",
	SourceDepthFlow:   "depthflow",
	SourceSentiment:   "sentiment",
	SourceCorrelation: "correlation",
	SourceCausal:      "causal",
	SourceLeadLag:     "leadlag",
	SourceLiquidity:   "liquidity",
	SourceExhaustion:  "exhaustion",
	SourcePrediction:  "prediction",
	SourceCVD:         "cvd",
	SourceToxicity:    "toxicity",
}

/*
String returns the source's dashboard name (empty for SourceNone).
*/
func (source SourceType) String() string {
	return sourceNames[source]
}

/*
Measurement is one classified signal reading in the market layer.

Strength is the raw fused signal (gauges only). SNR is always playbook sigma
units from FinalizeMeasurement — comparable across sources and categories.
Perspective branches gate on SNR; economic metrics (thesis_score, etc.) aggregate SNR.
*/
type Measurement struct {
	Symbol   string
	Source   SourceType
	Category CategoryType
	Strength float64 // raw fused strength for dashboards only
	SNR      float64 // adaptive sigma vs this series' history; 0 while warming up
	Last     float64 // last traded price, carried for the trader's sizing/fill
	Factors  []GaugeFactor
}
