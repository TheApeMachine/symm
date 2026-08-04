package types

import (
	"math"
	"time"
)

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

// Sample represents a fully populated market ticker payload point.
type Sample struct {
	Symbol    string    `json:"symbol"`
	Bid       float64   `json:"bid"`
	BidQty    float64   `json:"bid_qty"`
	Ask       float64   `json:"ask"`
	AskQty    float64   `json:"ask_qty"`
	Last      float64   `json:"last"`
	Volume    float64   `json:"volume"`
	// StepVolume is the quantity executed in this step alone, as opposed to
	// Volume, which is the cumulative traded quantity the ticker reports.
	StepVolume float64 `json:"step_volume"`
	VWAP      float64   `json:"vwap"`
	Low       float64   `json:"low"`
	High      float64   `json:"high"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	Timestamp time.Time `json:"timestamp"`
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

	/*
		IgnitionMove is the fraction the price gaps in a single step when the
		regime is entered, and IgnitionVolume is the multiple of normal
		executed volume that prints alongside it. Real pumps begin with one
		violent bar rather than a smooth ramp, and detectors score volume and
		price against their own recent medians, so a gradual climb never
		registers as ignition. IgnitionDecay is the fraction of the burst
		that survives each subsequent step.
	*/
	IgnitionMove   float64
	IgnitionVolume float64
	IgnitionDecay  float64
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

		/*
			Ignition describes how the target regime is entered, so it is
			taken from the target rather than blended away to nothing.
		*/
		IgnitionMove:   target.IgnitionMove,
		IgnitionVolume: target.IgnitionVolume,
		IgnitionDecay:  target.IgnitionDecay,
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
		Volatility:      0.05,
		SpreadScale:     1.0,
		BidAskAsymmetry: 1.0,
		BaseQty:         100.0,
		VolumeScale:     1.0,
		Cadence:         100 * time.Millisecond,
	},
	/*
		FastPump opens with one violent bar, modelled on an observed +30%
		30-minute candle that printed roughly eight times the surrounding
		volume, then continues to grind higher as the burst decays.
	*/
	FastPump: {
		Drift:           0.8,
		Volatility:      0.12,
		SpreadScale:     0.8,
		BidAskAsymmetry: 4.5,
		BaseQty:         500.0,
		VolumeScale:     5.0,
		Cadence:         20 * time.Millisecond,
		IgnitionMove:    0.30,
		IgnitionVolume:  8.0,
		IgnitionDecay:   0.6,
	},
	FastDump: {
		Drift:           -0.8,
		Volatility:      0.15,
		SpreadScale:     1.5,
		BidAskAsymmetry: 0.2,
		BaseQty:         600.0,
		VolumeScale:     6.0,
		Cadence:         15 * time.Millisecond,
		IgnitionMove:    -0.20,
		IgnitionVolume:  9.0,
		IgnitionDecay:   0.6,
	},
	VolumeAbsorption: {
		Drift:           0.02,
		Volatility:      0.02,
		SpreadScale:     0.5,
		BidAskAsymmetry: 0.1,
		BaseQty:         2500.0,
		VolumeScale:     10.0,
		Cadence:         50 * time.Millisecond,
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
}
