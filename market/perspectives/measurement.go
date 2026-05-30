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
}

/*
String returns the source's dashboard name (empty for SourceNone).
*/
func (source SourceType) String() string {
	return sourceNames[source]
}

/*
Measurement is one classified signal reading in the market layer.

The pipeline:

 1. Each signal (fluid, hawkes, pumpdump, …) publishes a Measurement with
    Source, Category (a DECISION.md row), Confidence, and SNR.
 2. Story and Perspective ingest those readings and match them to tree branches.
 3. A perspective tree is navigated only when the current measurement set
    contains the CategoryType required at each branch and SNR clears the branch
    threshold. Missing categories or SNR below the noise floor mean that path is
    not relevant.

Confidence is how completely the observation matches that category's criteria.
SNR is signal strength relative to the signal's own noise floor, computed in the
signal via numeric/adaptive.Ratio — not in perspectives.
*/
type Measurement struct {
	Symbol     string
	Source     SourceType
	Category   CategoryType
	Confidence float64
	SNR        float64
	Last       float64 // last traded price, carried for the trader's sizing/fill
}
