package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Decision records the action strategy selected and the alternatives it compared.
It is owned by one Thesis so intended behavior remains separate from execution.
*/
type Decision struct {
	ID      string    `json:"id" validate:"required"`
	Action  Action    `json:"action" validate:"required,oneof=enter|exit|reduce|hold|nothing"`
	Symbol  string    `json:"symbol" validate:"required"`
	At      time.Time `json:"at" validate:"required"`
	Utility float64   `json:"utility" validate:"finite"`
	// AllocationHaircut is 1−cognitive confidence (capital haircut), not a
	// calibrated loss probability. Prefer forecast Uncertainty for risk math.
	AllocationHaircut float64            `json:"allocation_haircut" validate:"finite,nonnegative"`
	Alternatives      map[string]float64 `json:"alternatives"`
	AllocationClass   string             `json:"allocationClass"`
	Opportunity       bool               `json:"opportunity"`
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
	AdverseSelection  *decimal.Decimal   `json:"adverseSelection" validate:"finite,nonnegative"`
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
	DisplacedQuantity *decimal.Decimal   `json:"displacedQuantity,omitempty"`
	DisplacedPrice    *decimal.Decimal   `json:"displacedPrice,omitempty"`
	ReservationID     string             `json:"reservationId,omitempty"`
	PositionStatus    Status             `json:"positionStatus,omitempty"`
	SellableQty       *decimal.Decimal   `json:"sellableQty,omitempty"`
	EntryAt           *time.Time         `json:"entryAt,omitempty"`
	ExitAt            *time.Time         `json:"exitAt,omitempty"`
	EntryPrice        *decimal.Decimal   `json:"entryPrice,omitempty"`
	EntryFee          *decimal.Decimal   `json:"entryFee,omitempty"`
	ExitPrice         *decimal.Decimal   `json:"exitPrice,omitempty"`
	ExitFee           *decimal.Decimal   `json:"exitFee,omitempty"`
	PnL               *decimal.Decimal   `json:"pnl,omitempty"`
	ReturnPct         *float64           `json:"returnPct,omitempty"`
	Mark              *decimal.Decimal   `json:"mark,omitempty"`
	Stoploss          *Stoploss          `json:"stoploss,omitempty"`
}

/*
NewDecision creates a Decision with a durable UUID assigned and the action
and symbol set. Callers fill remaining fields after construction.
*/
func NewDecision(action Action, symbol string) *Decision {
	return &Decision{
		ID:     uuid.NewString(),
		Action: action,
		Symbol: symbol,
		At:     time.Now().UTC(),
	}
}

/*
EnsureID assigns one UUID when the decision does not already carry its durable
position-link identifier.
*/
func (decision *Decision) EnsureID() {
	if decision == nil || decision.ID != "" {
		return
	}

	decision.ID = uuid.NewString()
}

/*
ValidID reports whether the decision identifier is a syntactically valid UUID.
*/
func (decision Decision) ValidID() bool {
	if decision.ID == "" {
		return false
	}

	_, err := uuid.Parse(decision.ID)
	return err == nil
}
