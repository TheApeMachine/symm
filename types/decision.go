package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Decision records the action strategy selected and the alternatives it compared.
It is owned by one Thesis so intended behavior remains separate from execution.
*/
type Decision struct {
	Action            Action             `json:"action" validate:"required,oneof=enter|exit|reduce|hold|nothing"`
	Symbol            string             `json:"symbol" validate:"required"`
	At                time.Time          `json:"at" validate:"required"`
	Utility           float64            `json:"utility" validate:"finite"`
	Risk              float64            `json:"risk" validate:"finite,nonnegative"`
	Alternatives      map[string]float64 `json:"alternatives"`
	AllocationClass   string             `json:"allocationClass"`
	ProposedNotional  *decimal.Decimal   `json:"proposedNotional" validate:"required"`
	ProposedQuantity  *decimal.Decimal   `json:"proposedQuantity" validate:"required"`
	ReferencePrice    *decimal.Decimal   `json:"referencePrice" validate:"required"`
	ValidThroughEpoch uint64             `json:"validThroughEpoch"`
	ForecastSource    string             `json:"forecastSource"`
	ForecastModel     string             `json:"forecastModel"`
	ForecastEpoch     uint64             `json:"forecastEpoch"`
	CalibrationCount  uint64             `json:"calibrationCount"`
	ExpectedReturn    *decimal.Decimal   `json:"expectedReturn" validate:"required"`
	ExpectedFees      *decimal.Decimal   `json:"expectedFees" validate:"required"`
	ExpectedSpread    *decimal.Decimal   `json:"expectedSpread" validate:"required"`
	ExpectedImpact    *decimal.Decimal   `json:"expectedImpact" validate:"required"`
	AdverseSelection  float64            `json:"adverseSelection" validate:"finite,nonnegative"`
	Uncertainty       float64            `json:"uncertainty" validate:"finite,nonnegative"`
	Confidence        float64            `json:"confidence" validate:"finite,min=0,max=1"`
	OpportunityMargin float64            `json:"opportunityMargin" validate:"finite"`
	CognitiveLead     float64            `json:"cognitiveLead" validate:"finite"`
	BasinConfidence   float64            `json:"basinConfidence" validate:"finite,nonnegative"`
	AvailableCapital  *decimal.Decimal   `json:"availableCapital" validate:"required"`
	OpenPositions     int                `json:"openPositions" validate:"min=0"`
	SlotCapacity      int                `json:"slotCapacity" validate:"min=0"`
	Cause             string             `json:"cause"`
	Reason            string             `json:"reason"`
	Displaces         string             `json:"displaces,omitempty"`
	ReservationID     string             `json:"reservationId,omitempty"`
}
