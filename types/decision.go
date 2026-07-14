package types

import "time"

/*
Decision records the action strategy selected and the alternatives it compared.
It is owned by one Thesis so intended behavior remains separate from execution.
*/
type Decision struct {
	Action            string             `json:"action"`
	Symbol            string             `json:"symbol"`
	At                time.Time          `json:"at"`
	Utility           float64            `json:"utility"`
	Alternatives      map[string]float64 `json:"alternatives"`
	AllocationClass   string             `json:"allocationClass"`
	ProposedNotional  float64            `json:"proposedNotional"`
	ProposedQuantity  float64            `json:"proposedQuantity"`
	ReferencePrice    float64            `json:"referencePrice"`
	ValidThroughEpoch uint64             `json:"validThroughEpoch"`
	ForecastSource    string             `json:"forecastSource"`
	ForecastModel     string             `json:"forecastModel"`
	ForecastEpoch     uint64             `json:"forecastEpoch"`
	CalibrationCount  uint64             `json:"calibrationCount"`
	ExpectedReturn    float64            `json:"expectedReturn"`
	ExpectedFees      float64            `json:"expectedFees"`
	ExpectedSpread    float64            `json:"expectedSpread"`
	ExpectedImpact    float64            `json:"expectedImpact"`
	AdverseSelection  float64            `json:"adverseSelection"`
	Uncertainty       float64            `json:"uncertainty"`
	Confidence        float64            `json:"confidence"`
	AvailableCapital  float64            `json:"availableCapital"`
	OpenPositions     int                `json:"openPositions"`
	SlotCapacity      int                `json:"slotCapacity"`
	Cause             string             `json:"cause"`
	Reason            string             `json:"reason"`
}
