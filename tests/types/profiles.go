package types

import (
	"time"

	coretypes "github.com/theapemachine/symm/types"
)

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
					Metric:            coretypes.MetricPrecursor,
					Side:              coretypes.SideBuy,
					MinimumNormalized: PositiveEvidenceFloor,
				},
				{
					Metric:            coretypes.MetricRVOL,
					Side:              coretypes.SideNone,
					MinimumNormalized: NormalizedEmpiricalRatioBaseline,
				},
				{
					Metric:            coretypes.MetricCompression,
					Side:              coretypes.SideNone,
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
	// MeanReversion diffuses the latent price while continuously pulling it
	// back toward the scenario's opening anchor.
	MeanReversion: {
		Diffusion:       0.25,
		MeanReversion:   1.5,
		Volatility:      0.1,
		SpreadScale:     1.2,
		BidAskAsymmetry: 1.0,
		BaseQty:         150.0,
		VolumeScale:     1.5,
		Cadence:         50 * time.Millisecond,
	},
	// FalseBreakout prints one upward discontinuity, then pulls the permanent
	// path back to its anchor instead of leaking a future reversal label.
	FalseBreakout: {
		Volatility:      0.15,
		SpreadScale:     1.4,
		MeanReversion:   2.0,
		BidAskAsymmetry: 2.0,
		BaseQty:         250.0,
		VolumeScale:     3.0,
		Cadence:         25 * time.Millisecond,
		AggressorSide:   "buy",
		IgnitionMove:    0.05,
		IgnitionVolume:  4.0,
		IgnitionDecay:   0.0,
	},
	// Whipsaw alternates equal permanent moves around a noisy tape, challenging
	// logic that mistakes repeated breaks for a durable trend.
	Whipsaw: {
		Volatility:      0.2,
		SpreadScale:     1.8,
		OscillationMove: 0.01,
		BidAskAsymmetry: 1.0,
		BaseQty:         180.0,
		VolumeScale:     2.5,
		Cadence:         50 * time.Millisecond,
	},
	// RandomWalk is a zero-drift null with permanent independent increments.
	RandomWalk: {
		Diffusion:       0.4,
		Volatility:      0.05,
		SpreadScale:     1.0,
		BidAskAsymmetry: 1.0,
		BaseQty:         100.0,
		VolumeScale:     1.0,
		Cadence:         100 * time.Millisecond,
	},
	// SpreadOnly moves transaction cost without introducing directional edge.
	SpreadOnly: {
		SpreadScale:     2.0,
		SpreadJitter:    0.8,
		BidAskAsymmetry: 1.0,
		BaseQty:         100.0,
		VolumeScale:     1.0,
		Cadence:         100 * time.Millisecond,
	},
}
