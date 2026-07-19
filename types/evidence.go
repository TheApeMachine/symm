package types

/*
StopEvidence is the thin numeric projection Stoploss consumes from one Thesis
cut (or a mark-only tick). Missing input leaves Present false so the regulator
freezes instead of inventing prices.
*/
type StopEvidence struct {
	Symbol               string
	Mark                 float64
	Entry                float64
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
	SellCapacity         float64
	// RetreatPressure is cancelled touch qty / prior touch (toxicity). When
	// positive, mark moves are quote-only and must not drive stop geometry.
	RetreatPressure float64
	Present         bool
}
