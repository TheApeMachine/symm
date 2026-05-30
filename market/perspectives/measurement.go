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

SNR is signal strength relative to the signal's own noise floor (numeric/adaptive).
Perspective branches compare Measurement.SNR to the unitless playbook floor.
*/
type Measurement struct {
	Symbol   string
	Source   SourceType
	Category CategoryType
	SNR      float64
	Last     float64 // last traded price, carried for the trader's sizing/fill
}
