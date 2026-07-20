package types

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
StopEvidence is the thin numeric projection Stoploss consumes from one Thesis
cut (or a mark-only tick). Missing input leaves Present false so the regulator
freezes instead of inventing prices.
*/
type StopEvidence struct {
	Symbol               string
	Mark                 float64
	Entry                float64
	ReferencePrice       *decimal.Decimal
	ForecastEpoch        uint64
	NormalizedResidual   float64
	ExpectedReturn       float64
	Uncertainty          float64
	IncrementalMSE       float64
	ReturnReady          bool
	CausalReady          bool
	CausalExpectedReturn float64
	CognitionReady       bool
	CognitionConfidence  float64
	CognitionWinner      string
	CognitionAmbiguous   bool
	Spread               float64
	SellCapacity         *decimal.Decimal
	// RetreatPressure is cancelled touch qty / prior touch (toxicity). When
	// positive, mark moves are quote-only and must not drive stop geometry.
	RetreatPressure float64
	// RetreatReady is true when this cut observed a retreat measurement so
	// Stoploss may latch or clear pressure; absent cuts leave sticky retreat.
	RetreatReady bool
	Present      bool
}
