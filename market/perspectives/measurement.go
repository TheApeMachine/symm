package perspectives

import "time"

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

SNR is temporal surprise: how many standard deviations the current category
standout — margin by which the assigned category beat its alternatives — stands
above this symbol's running standout baseline. It answers "how decisively did
this category win versus its own recent history," not how deep inside a band the
reading sits. Perspective branches gate on SNR (UnitSNR); UnitConfidence gates on
the instantaneous band clarity instead.
*/
type Measurement struct {
	At         time.Time `json:"at,omitempty"`
	Symbol     string
	Source     SourceType
	Category   CategoryType
	Strength   float64     // raw fused strength for dashboards only
	Confidence float64     // cross-sectional band margin; 0 on a boundary
	SNR        float64     // temporal surprise: sigma above this symbol's own recent standout floor
	Last       float64     // last traded price, carried for the trader's sizing/fill
	Volume     float64     // quote-currency notional volume when known (ticker volume × last)
	SpreadBPS  float64     // quoted spread in basis points when bid/ask are known; 0 falls back to static replay slippage
	Bid        float64     `json:"bid,omitempty"`
	Ask        float64     `json:"ask,omitempty"`
	BookBids   []BookLevel `json:"book_bids,omitempty"` // L2 bid depth at capture time for replay fills
	BookAsks   []BookLevel `json:"book_asks,omitempty"` // L2 ask depth at capture time for replay fills
}
