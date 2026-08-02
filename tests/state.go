package tests

import (
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
	VWAP      float64   `json:"vwap"`
	Low       float64   `json:"low"`
	High      float64   `json:"high"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	Timestamp time.Time `json:"timestamp"`
}

// Order represents a single Level3 book order.
type Order struct {
	ID       string
	Price    float64
	Qty      float64
	Priority int
	At       time.Time
}

// Fill represents an executed trade fill.
type Fill struct {
	Price     float64
	Qty       float64
	Timestamp time.Time
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
	FastPump: {
		Drift:           0.8,
		Volatility:      0.12,
		SpreadScale:     0.8,
		BidAskAsymmetry: 4.5,
		BaseQty:         500.0,
		VolumeScale:     5.0,
		Cadence:         20 * time.Millisecond,
	},
	FastDump: {
		Drift:           -0.8,
		Volatility:      0.15,
		SpreadScale:     1.5,
		BidAskAsymmetry: 0.2,
		BaseQty:         600.0,
		VolumeScale:     6.0,
		Cadence:         15 * time.Millisecond,
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
