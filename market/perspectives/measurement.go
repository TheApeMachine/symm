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

Strength is the raw fused signal (gauges only).

Confidence is cross-sectional clarity: how decisively the reading sits inside its
assigned category band right now — margin to the nearest boundary, 0 on a boundary.
It says nothing about which category won; a clear StochasticNoise reads high.

SNR is temporal surprise: how many of its own recent standard deviations the
current Confidence stands above this symbol's running clarity baseline. A signal
that is always equally clear reads ~0; a sudden jump in clarity spikes. It answers
"how clear is this selection versus its own recent history," which is the noise-floor
sense of signal-to-noise. Perspective branches gate on SNR (UnitSNR); UnitConfidence
gates on the instantaneous clarity instead.
*/
type Measurement struct {
	Symbol     string
	Source     SourceType
	Category   CategoryType
	Strength   float64 // raw fused strength for dashboards only
	Confidence float64 // cross-sectional band margin; 0 on a boundary
	SNR        float64 // temporal surprise: sigma above this symbol's own recent clarity floor
	Last       float64 // last traded price, carried for the trader's sizing/fill
}
