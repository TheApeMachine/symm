package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/nomagique/learning"
)

/*
Decision records the action strategy selected and the alternatives it compared.
It is owned by one Thesis so intended behavior remains separate from execution.
*/
type Decision struct {
	ID         string    `json:"id" validate:"required"`
	Action     Action    `json:"action" validate:"required,oneof=enter|exit|reduce|hold|nothing"`
	Symbol     string    `json:"symbol" validate:"required"`
	At         time.Time `json:"at" validate:"required"`
	Utility    float64   `json:"utility" validate:"finite"`
	GraphScore float64   `json:"graphScore" validate:"finite"`
	// AllocationHaircut is the fraction removed from the pre-risk notional by
	// adverse-selection, toxicity, and executable-liquidity evidence. It is an
	// allocation penalty, not a calibrated loss probability.
	AllocationHaircut       float64             `json:"allocation_haircut" validate:"finite,min=0,max=1"`
	AllocationHaircutReason string              `json:"allocation_haircut_reason,omitempty"`
	Alternatives            map[string]float64  `json:"alternatives"`
	AllocationClass         string              `json:"allocationClass"`
	Opportunity             bool                `json:"opportunity"`
	ProposedNotional        *decimal.Decimal    `json:"proposedNotional" validate:"required"`
	ProposedQuantity        *decimal.Decimal    `json:"proposedQuantity" validate:"required"`
	ReferencePrice          *decimal.Decimal    `json:"referencePrice" validate:"required"`
	ValidThroughEpoch       uint64              `json:"validThroughEpoch"`
	ArbitrationRound        int64               `json:"arbitrationRound"`
	ForecastSource          string              `json:"forecastSource"`
	ForecastModel           string              `json:"forecastModel"`
	ForecastEpoch           uint64              `json:"forecastEpoch"`
	Forecast                *learning.RLSOutput `json:"forecast,omitempty"`
	ForecastHorizon         int                 `json:"forecastHorizon" validate:"min=0"`
	CalibrationCount        uint64              `json:"calibrationCount"`
	ExpectedReturn          *decimal.Decimal    `json:"expectedReturn" validate:"required"`
	ExpectedFees            *decimal.Decimal    `json:"expectedFees" validate:"required"`
	ExpectedSpread          *decimal.Decimal    `json:"expectedSpread" validate:"required"`
	ExpectedImpact          *decimal.Decimal    `json:"expectedImpact" validate:"required"`
	AdverseSelection        *decimal.Decimal    `json:"adverseSelection" validate:"finite,nonnegative"`
	Uncertainty             float64             `json:"uncertainty" validate:"finite,nonnegative"`
	Confidence              float64             `json:"confidence" validate:"finite,min=0,max=1"`
	CausalPrecision         float64             `json:"causalPrecision" validate:"finite,min=0,max=1"`
	OpportunityMargin       float64             `json:"opportunityMargin" validate:"finite"`
	CognitiveLead           float64             `json:"cognitiveLead" validate:"finite"`
	BasinConfidence         float64             `json:"basinConfidence" validate:"finite,nonnegative"`
	AvailableCapital        *decimal.Decimal    `json:"availableCapital" validate:"required"`
	OpenPositions           int                 `json:"openPositions" validate:"min=0"`
	SlotCapacity            int                 `json:"slotCapacity" validate:"min=0"`
	Cause                   string              `json:"cause"`
	Reason                  string              `json:"reason"`
	Displaces               string              `json:"displaces,omitempty"`
	DisplacedQuantity       *decimal.Decimal    `json:"displacedQuantity,omitempty"`
	DisplacedPrice          *decimal.Decimal    `json:"displacedPrice,omitempty"`
	ReservationID           string              `json:"reservationId,omitempty"`
	PositionStatus          Status              `json:"positionStatus,omitempty"`
	SellableQty             *decimal.Decimal    `json:"sellableQty,omitempty"`
	EntryAt                 *time.Time          `json:"entryAt,omitempty"`
	ExitAt                  *time.Time          `json:"exitAt,omitempty"`
	EntryPrice              *decimal.Decimal    `json:"entryPrice,omitempty"`
	EntryFee                *decimal.Decimal    `json:"entryFee,omitempty"`
	ExitPrice               *decimal.Decimal    `json:"exitPrice,omitempty"`
	ExitFee                 *decimal.Decimal    `json:"exitFee,omitempty"`
	PnL                     *decimal.Decimal    `json:"pnl,omitempty"`
	ReturnPct               *float64            `json:"returnPct,omitempty"`
	Mark                    *decimal.Decimal    `json:"mark,omitempty"`
	Stoploss                *Stoploss           `json:"stoploss,omitempty"`
	/*
		Risk is the stop geometry this entry was sized under. It travels with
		the decision because the quantity above was derived from it: the
		allocator capped the size so that the distance to the hard floor costs
		no more than the loss budget, and a stop later placed at some other
		distance would silently undo that.
	*/
	Risk RiskPlan `json:"risk"`
	/*
		Trace records the evaluator path that produced an opportunity decision.
		It is absent on continuation and execution-only decisions because those
		paths do not run the entry trajectory search.
	*/
	Trace *DecisionTrace `json:"trace,omitempty"`
}

/*
DecisionTrace is the observable entry-decision chain. It carries values already
used by strategy rather than a frontend reconstruction of those values.
*/
type DecisionTrace struct {
	GraphSupports    float64           `json:"graphSupports"`
	GraphContradicts float64           `json:"graphContradicts"`
	MCTS             DecisionMCTSTrace `json:"mcts"`
}

/*
DecisionMCTSBranch is one root child actually explored by the causal search:
the action it represents, how many simulations visited it, and the mean
reward those simulations produced. Visits and MeanReward come straight off
the search tree — nothing here is a frontend reconstruction of the search.
*/
type DecisionMCTSBranch struct {
	Action     string  `json:"action"`
	Visits     int     `json:"visits"`
	MeanReward float64 `json:"meanReward"`
}

/*
DecisionMCTSTrace records every root branch the causal search actually
explored, ranked by visit count. The recommended action is the branch with
the most visits (the search's own robust-child rule), not a value comparison
computed separately by the frontend.
*/
type DecisionMCTSTrace struct {
	Iterations        int                  `json:"iterations"`
	Branches          []DecisionMCTSBranch `json:"branches"`
	RecommendedAction string               `json:"recommendedAction,omitempty"`
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
