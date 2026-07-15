package types

import "time"

/*
Decision records the action strategy selected and the alternatives it compared.
It is owned by one Thesis so intended behavior remains separate from execution.
*/
type Decision struct {
	Action            string             `json:"action" validate:"required,oneof=enter|exit|reduce|hold|nothing"`
	Symbol            string             `json:"symbol" validate:"required"`
	At                time.Time          `json:"at" validate:"required"`
	Utility           float64            `json:"utility" validate:"finite"`
	Alternatives      map[string]float64 `json:"alternatives"`
	AllocationClass   string             `json:"allocationClass"`
	ProposedNotional  float64            `json:"proposedNotional" validate:"finite"`
	ProposedQuantity  float64            `json:"proposedQuantity" validate:"finite"`
	ReferencePrice    float64            `json:"referencePrice" validate:"finite"`
	ValidThroughEpoch uint64             `json:"validThroughEpoch"`
	ForecastSource    string             `json:"forecastSource"`
	ForecastModel     string             `json:"forecastModel"`
	ForecastEpoch     uint64             `json:"forecastEpoch"`
	CalibrationCount  uint64             `json:"calibrationCount"`
	ExpectedReturn    float64            `json:"expectedReturn" validate:"finite"`
	ExpectedFees      float64            `json:"expectedFees" validate:"finite,nonnegative"`
	ExpectedSpread    float64            `json:"expectedSpread" validate:"finite,nonnegative"`
	ExpectedImpact    float64            `json:"expectedImpact" validate:"finite,nonnegative"`
	AdverseSelection  float64            `json:"adverseSelection" validate:"finite,nonnegative"`
	Uncertainty       float64            `json:"uncertainty" validate:"finite,nonnegative"`
	Confidence        float64            `json:"confidence" validate:"finite,min=0,max=1"`
	AvailableCapital  float64            `json:"availableCapital" validate:"finite,nonnegative"`
	OpenPositions     int                `json:"openPositions" validate:"min=0"`
	SlotCapacity      int                `json:"slotCapacity" validate:"min=0"`
	Cause             string             `json:"cause"`
	Reason            string             `json:"reason"`
}
