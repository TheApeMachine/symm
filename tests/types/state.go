package types

import (
	"math"
	"time"

	coretypes "github.com/theapemachine/symm/types"
)

const (
	QuantityJitterMinimum  = 0.8
	QuantityJitterMaximum  = 1.2
	VolumeJitterMinimum    = 0.5
	VolumeJitterMaximum    = 1.5
	EmpiricalRatioBaseline = 1.0
	PositiveEvidenceFloor  = 0.0
)

/*
PrecursorMetricExpectation declares evidence the regime is designed to
produce before its discontinuous event is sampled. The normalized floor is a
domain boundary owned by the fixture profile, not an assertion invented by an
integration test.
*/
type PrecursorMetricExpectation struct {
	Metric            coretypes.MetricType
	Side              coretypes.MeasurementSide
	MinimumNormalized float64
}

/*
PrecursorContract is the analytical contract paired with a generated regime.
It names only decision-facing evidence that the profile deliberately creates.
*/
type PrecursorContract struct {
	MinimumObservations int
	Metrics             []PrecursorMetricExpectation
	Categories          []coretypes.CategoryType
}

/*
PrecursorExpectation resolves the profile contract into bounds implied by the
same quantity jitter used by Generator. This keeps generation and assertions
on one source of truth.
*/
type PrecursorExpectation struct {
	MinimumStepVolume          float64
	MaximumBaselineStepVolume  float64
	MinimumBidQuantity         float64
	MaximumBaselineBidQuantity float64
	MaximumAskQuantity         float64
	MinimumBaselineAskQuantity float64
	Contract                   PrecursorContract
}

type MarketState int

const (
	Baseline MarketState = iota
	FastPump
	SlowPump
	FastDump
	SlowDump
	VolumeAbsorption
	SpreadCompression
	ThinLiquidity
	LoadedLiquidity
	SpoofLiquidity
	FlashCrash
	SidewaysChop
	VolatilitySpike
)

/*
MomentumMap describes how forcefully a market enters each state. The sign is
the direction of the move, so transition speed is the magnitude: a flash
crash (-1.2) arrives faster than a slow bleed (-0.3). States that describe a
condition rather than a move carry no momentum of their own and settle at the
baseline pace.
*/
var MomentumMap = map[MarketState]float64{
	Baseline:          0.0,
	FastPump:          0.9,
	SlowPump:          0.3,
	FastDump:          -0.9,
	SlowDump:          -0.3,
	VolumeAbsorption:  0.1,
	SpreadCompression: 0.0,
	ThinLiquidity:     0.0,
	LoadedLiquidity:   0.0,
	SpoofLiquidity:    0.0,
	FlashCrash:        -1.2,
	SidewaysChop:      0.0,
	VolatilitySpike:   0.0,
}

/*
Sample represents a fully populated market ticker payload point.
*/
type Sample struct {
	Symbol        string  `json:"symbol"`
	AggressorSide string  `json:"-"`
	Bid           float64 `json:"bid"`
	BidQty        float64 `json:"bid_qty"`
	Ask           float64 `json:"ask"`
	AskQty        float64 `json:"ask_qty"`
	Last          float64 `json:"last"`
	Volume        float64 `json:"volume"`
	// StepVolume is the quantity executed in this step alone, as opposed to
	// Volume, which is the cumulative traded quantity the ticker reports.
	StepVolume float64   `json:"step_volume"`
	VWAP       float64   `json:"vwap"`
	Low        float64   `json:"low"`
	High       float64   `json:"high"`
	Change     float64   `json:"change"`
	ChangePct  float64   `json:"change_pct"`
	Timestamp  time.Time `json:"timestamp"`
}

// RegimeProfile controls the physical behavior of the ticker parameters.
type RegimeProfile struct {
	Drift           float64       // Directional push per step (-1.0 to +1.0)
	Volatility      float64       // Price variance scaling
	SpreadScale     float64       // Spread multiplier (1.0 = normal, 0.1 = compressed, 5.0 = wide)
	BidAskAsymmetry float64       // Ratio of Bid Qty vs Ask Qty (>1 = Bid heavy, <1 = Ask heavy)
	BaseQty         float64       // Average order size base
	VolumeScale     float64       // Trade volume surge factor
	Cadence         time.Duration // Time interval step between ticks
	AggressorSide   string        // Explicit taker intent when the regime owns flow direction

	// IgnitionMove is the fraction the price gaps in a single step when the
	// regime is entered, and IgnitionVolume is the multiple of normal
	// executed volume that prints alongside it. Real pumps begin with one
	// violent bar rather than a smooth ramp, and detectors score volume and
	// price against their own recent medians, so a gradual climb never
	// registers as ignition. IgnitionDecay is the fraction of the burst
	// that survives each subsequent step.
	IgnitionMove   float64
	IgnitionVolume float64
	IgnitionDecay  float64
	Precursor      PrecursorContract

	// AdmitsLong states whether this regime is one a long position belongs in.
	// It describes the generated market rather than any stack that reads it: a
	// pump winding up is an opportunity to be long and a crash is not, and that
	// remains true of the prices in this file whatever code observes them.

	// Two of these regimes are only worth generating because of this field.
	// SpoofLiquidity stacks the bid harder than the pump does while almost
	// nothing trades, and ThinLiquidity widens the spread on a book with
	// nothing behind it — both present the depth signature of an opportunity
	// without the flow, so a stack that reads depth alone takes them and one
	// that reads what is executable does not.
	AdmitsLong bool
}

/*
PrecursorExpectation derives observable precursor bounds from this profile
and the baseline it is leaving.
*/
func (profile RegimeProfile) PrecursorExpectation(
	baseline RegimeProfile,
) PrecursorExpectation {
	return PrecursorExpectation{
		MinimumStepVolume: profile.BaseQty * profile.VolumeScale *
			VolumeJitterMinimum,
		MaximumBaselineStepVolume: baseline.BaseQty * baseline.VolumeScale *
			VolumeJitterMaximum,
		MinimumBidQuantity: profile.BaseQty * profile.BidAskAsymmetry *
			QuantityJitterMinimum,
		MaximumBaselineBidQuantity: baseline.BaseQty * baseline.BidAskAsymmetry *
			QuantityJitterMaximum,
		MaximumAskQuantity: profile.BaseQty / profile.BidAskAsymmetry *
			QuantityJitterMaximum,
		MinimumBaselineAskQuantity: baseline.BaseQty / baseline.BidAskAsymmetry *
			QuantityJitterMinimum,
		Contract: profile.Precursor,
	}
}

/*
Blend interpolates between two regime profiles, where progress runs from 0
(entirely source) to 1 (entirely target).

Additive quantities move linearly. Multipliers, quantities and cadence move
geometrically, because a spread widening from 1.0 to 8.0 passes through 2.8
at the halfway point, not 4.5 — a market does not become "half as liquid" on
a linear scale. Geometric blending also keeps every strictly positive field
positive throughout the transition.
*/
func Blend(source, target RegimeProfile, progress float64) RegimeProfile {
	progress = max(0.0, min(1.0, progress))

	return RegimeProfile{
		Drift:           lerp(source.Drift, target.Drift, progress),
		Volatility:      lerp(source.Volatility, target.Volatility, progress),
		SpreadScale:     geolerp(source.SpreadScale, target.SpreadScale, progress),
		BidAskAsymmetry: geolerp(source.BidAskAsymmetry, target.BidAskAsymmetry, progress),
		BaseQty:         geolerp(source.BaseQty, target.BaseQty, progress),
		VolumeScale:     geolerp(source.VolumeScale, target.VolumeScale, progress),
		Cadence: time.Duration(geolerp(
			float64(source.Cadence), float64(target.Cadence), progress,
		)),
		AggressorSide: target.AggressorSide,

		// Ignition describes how the target regime is entered, so it is
		// taken from the target rather than blended away to nothing.
		IgnitionMove:   target.IgnitionMove,
		IgnitionVolume: target.IgnitionVolume,
		IgnitionDecay:  target.IgnitionDecay,
		Precursor:      target.Precursor,
		AdmitsLong:     target.AdmitsLong,
	}
}

// lerp interpolates linearly, for quantities that are signed or additive.
func lerp(source, target, progress float64) float64 {
	return source + (target-source)*progress
}

/*
geolerp interpolates geometrically, for strictly positive scaling factors.
Values that are zero or negative have no meaningful ratio, so those fall
back to linear interpolation.
*/
func geolerp(source, target, progress float64) float64 {
	if source <= 0 || target <= 0 {
		return lerp(source, target, progress)
	}

	return source * math.Pow(target/source, progress)
}

var DefaultProfiles = map[MarketState]RegimeProfile{
	Baseline: {
		Drift:           0.0,
		Volatility:      0.0,
		SpreadScale:     1.0,
		BidAskAsymmetry: 1.0,
		BaseQty:         100.0,
		VolumeScale:     1.0,
		Cadence:         100 * time.Millisecond,
		Precursor: PrecursorContract{
			// A level and two changes are the minimum empirical baseline.
			MinimumObservations: 3,
		},
	},
	// FastPump opens with one violent bar, modelled on an observed +30%
	// 30-minute candle that printed roughly eight times the surrounding
	// volume, then continues to grind higher as the burst decays.
	FastPump: {
		Drift:           0.8,
		Volatility:      0.04,
		SpreadScale:     0.8,
		BidAskAsymmetry: 8.0,
		BaseQty:         500.0,
		VolumeScale:     5.0,
		Cadence:         20 * time.Millisecond,
		AggressorSide:   "buy",
		IgnitionMove:    0.30,
		IgnitionVolume:  8.0,
		IgnitionDecay:   0.6,
		AdmitsLong:      true,
		Precursor: PrecursorContract{
			MinimumObservations: 2,
			Metrics: []PrecursorMetricExpectation{
				{
					Metric:            coretypes.MetricIgnition,
					Side:              coretypes.SideBuy,
					MinimumNormalized: EmpiricalRatioBaseline,
				},
				{
					Metric:            coretypes.MetricCompression,
					Side:              coretypes.SideBuy,
					MinimumNormalized: PositiveEvidenceFloor,
				},
			},
			Categories: []coretypes.CategoryType{
				coretypes.VerticalIgnition,
				coretypes.CoiledCompression,
			},
		},
	},
	FastDump: {
		Drift:           -0.8,
		Volatility:      0.15,
		SpreadScale:     1.5,
		BidAskAsymmetry: 0.2,
		BaseQty:         600.0,
		VolumeScale:     6.0,
		Cadence:         15 * time.Millisecond,
		AggressorSide:   "sell",
		IgnitionMove:    -0.20,
		IgnitionVolume:  9.0,
		IgnitionDecay:   0.6,
	},
	// SlowPump is persistent buy-side pressure without a discontinuous event.
	// It deliberately carries no ignition and does not admit the fast-pump
	// entry contract.
	SlowPump: {
		Drift:           0.25,
		Volatility:      0.03,
		SpreadScale:     0.9,
		BidAskAsymmetry: 2.5,
		BaseQty:         200.0,
		VolumeScale:     1.5,
		Cadence:         100 * time.Millisecond,
		AggressorSide:   "buy",
	},
	// SlowDump is the sell-side counterpart: sustained pressure without the
	// gap and volume burst that define a crash or fast dump.
	SlowDump: {
		Drift:           -0.25,
		Volatility:      0.03,
		SpreadScale:     1.1,
		BidAskAsymmetry: 0.4,
		BaseQty:         200.0,
		VolumeScale:     1.5,
		Cadence:         100 * time.Millisecond,
		AggressorSide:   "sell",
	},
	VolumeAbsorption: {
		Drift:           0.02,
		Volatility:      0.02,
		SpreadScale:     0.5,
		BidAskAsymmetry: 0.1,
		BaseQty:         2500.0,
		VolumeScale:     10.0,
		Cadence:         50 * time.Millisecond,
		AggressorSide:   "buy",
	},
	SpreadCompression: {
		Drift:           0.0,
		Volatility:      0.01,
		SpreadScale:     0.1,
		BidAskAsymmetry: 1.0,
		BaseQty:         100.0,
		VolumeScale:     0.5,
		Cadence:         100 * time.Millisecond,
	},
	// LoadedLiquidity keeps a deep, balanced book around an ordinary tape so
	// depth alone cannot be mistaken for directional demand.
	LoadedLiquidity: {
		Drift:           0.0,
		Volatility:      0.02,
		SpreadScale:     0.5,
		BidAskAsymmetry: 1.0,
		BaseQty:         5000.0,
		VolumeScale:     1.0,
		Cadence:         100 * time.Millisecond,
	},
	ThinLiquidity: {
		Drift:           0.0,
		Volatility:      0.25,
		SpreadScale:     3.5,
		BidAskAsymmetry: 1.0,
		BaseQty:         5.0,
		VolumeScale:     0.1,
		Cadence:         250 * time.Millisecond,
	},
	FlashCrash: {
		Drift:           -2.5,
		Volatility:      0.80,
		SpreadScale:     8.0,
		BidAskAsymmetry: 0.02,
		BaseQty:         1000.0,
		VolumeScale:     15.0,
		Cadence:         5 * time.Millisecond,
	},
	SpoofLiquidity: {
		Drift:           0.0,
		Volatility:      0.03,
		SpreadScale:     1.0,
		BidAskAsymmetry: 12.0,
		BaseQty:         5000.0,
		VolumeScale:     0.2,
		Cadence:         50 * time.Millisecond,
	},
	// SidewaysChop alternates around a stationary center on ordinary volume;
	// its variance is real, but it has no directional flow contract.
	SidewaysChop: {
		Drift:           0.0,
		Volatility:      0.4,
		SpreadScale:     1.5,
		BidAskAsymmetry: 1.0,
		BaseQty:         150.0,
		VolumeScale:     2.0,
		Cadence:         50 * time.Millisecond,
	},
	// VolatilitySpike raises dispersion and tape rate together while keeping
	// direction balanced, separating activity from an executable long thesis.
	VolatilitySpike: {
		Drift:           0.0,
		Volatility:      1.0,
		SpreadScale:     2.0,
		BidAskAsymmetry: 1.0,
		BaseQty:         300.0,
		VolumeScale:     8.0,
		Cadence:         10 * time.Millisecond,
	},
}
