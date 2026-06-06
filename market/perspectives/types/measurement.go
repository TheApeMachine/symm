package types

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
)

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

Confidence is the signal's confidence in its own category selection: how decisively
it picked this category for this reading. Its honest floor is 1/N — a uniform guess
among the signal's N categories — and it is never clamped or fused with SNR. It says
nothing about which category won; a confident StochasticNoise reads high.

SNR is temporal surprise, orthogonal to confidence: how many standard deviations
the Shannon surprisal of the selected category stands above this symbol's
running surprisal baseline. It answers "how unexpected is this category selection
compared to recent history," not how clearly the observation sits in its band.
A reading can pair low confidence with high SNR (surprising but unsure) — still
low trust. Perspective branches gate on SNR (UnitSNR); UnitConfidence gates on the
selection confidence instead.
*/
type Measurement struct {
	At         time.Time `json:"at,omitempty"`
	Symbol     string
	Source     SourceType
	Category   CategoryType
	Strength   float64     // raw fused strength for dashboards only
	Confidence float64     // cross-sectional band margin; 0 on a boundary
	SNR        float64     // temporal surprise: sigma above this symbol's own recent surprisal baseline
	Last       float64     // last traded price, carried for the trader's sizing/fill
	Volume     float64     // quote-currency notional volume when known (ticker volume × last)
	SpreadBPS  float64     // quoted spread in basis points when bid/ask are known; 0 falls back to static replay slippage
	Bid        float64     `json:"bid,omitempty"`
	Ask        float64     `json:"ask,omitempty"`
	BookBids   []BookLevel `json:"book_bids,omitempty"` // L2 bid depth at capture time for replay fills
	BookAsks   []BookLevel `json:"book_asks,omitempty"` // L2 ask depth at capture time for replay fills
}

func NewMeasurement(
	symbol string,
	source SourceType,
	category CategoryType,
	strength float64,
	confidence float64,
	snr float64,
	last float64,
) Measurement {
	return Measurement{
		Symbol:     symbol,
		Source:     source,
		Category:   category,
		Strength:   strength,
		Confidence: confidence,
		SNR:        snr,
		Last:       last,
	}
}

func (measurement *Measurement) Send(pool *qpool.Q) error {
	// strength and snr are intentionally NOT required: a warm-up / neutral reading
	// legitimately carries strength 0 (no fused signal yet) and snr 0 (no surprise
	// yet) while still being a valid, always-emitted Measurement. Identity, the
	// selected category, the selection confidence, and the price must be present.
	if err := errnie.Require(map[string]any{
		"symbol":     measurement.Symbol,
		"source":     measurement.Source,
		"category":   measurement.Category,
		"confidence": measurement.Confidence,
		"last":       measurement.Last,
	}); err != nil {
		return errnie.Error(err, "%s measurement for %q", measurement.Source.String(), measurement.Symbol)
	}

	bus.Group(
		pool, "measurements", viper.GetDuration("system.queue.ttl"),
	).Send(&qpool.QValue[any]{
		Type:  "measurement",
		Value: *measurement,
	})

	return nil
}
