package types

import (
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
EntryCost is the current, observable execution boundary for one proposed long.
It states only facts available at admission time: visible entry VWAP, the
crossing costs paid now, and the sale price that would recover both known fees.
It deliberately contains no future price, future spread, or expected return.
*/
type EntryCost struct {
	EntryPrice         *decimal.Decimal `json:"entryPrice,omitempty"`
	BestAsk            *decimal.Decimal `json:"bestAsk,omitempty"`
	BestBid            *decimal.Decimal `json:"bestBid,omitempty"`
	Midpoint           *decimal.Decimal `json:"midpoint,omitempty"`
	GrossNotional      *decimal.Decimal `json:"grossNotional,omitempty"`
	EntryFee           *decimal.Decimal `json:"entryFee,omitempty"`
	ExitFeeAtBreakEven *decimal.Decimal `json:"exitFeeAtBreakEven,omitempty"`
	RoundTripFees      *decimal.Decimal `json:"roundTripFees,omitempty"`
	Spread             *decimal.Decimal `json:"spread,omitempty"`
	Impact             *decimal.Decimal `json:"impact,omitempty"`
	BreakEven          *decimal.Decimal `json:"breakEven,omitempty"`
}

/*
Decision records the action strategy selected and the alternatives it compared.
The immutable decision identifier links admission, execution, and Hindsight.
*/
type Decision struct {
	ID               string              `json:"id" validate:"required"`
	Action           Action              `json:"action" validate:"required,oneof=enter|exit|scale|reduce|hold|nothing"`
	Symbol           string              `json:"symbol" validate:"required"`
	At               time.Time           `json:"at" validate:"required"`
	Direction        float64             `json:"direction" validate:"finite,min=-1,max=1"`
	Alternatives     map[string]float64  `json:"alternatives"`
	AllocationClass  string              `json:"allocationClass"`
	Opportunity      bool                `json:"opportunity"`
	OpportunityType  string              `json:"opportunityType,omitempty"`
	OpportunityPhase string              `json:"opportunityPhase,omitempty"`
	PredictiveReady  bool                `json:"predictiveReady"`
	PredictiveStatus string              `json:"predictiveStatus"`
	TaskSkill        float64             `json:"taskSkill" validate:"finite,nonnegative"`
	TaskSkillReady   bool                `json:"taskSkillReady"`
	ProposedNotional *decimal.Decimal    `json:"proposedNotional" validate:"required"`
	ProposedQuantity *decimal.Decimal    `json:"proposedQuantity" validate:"required"`
	ReferencePrice   *decimal.Decimal    `json:"referencePrice" validate:"required"`
	ForecastSource   string              `json:"forecastSource"`
	ForecastModel    string              `json:"forecastModel"`
	Forecast         *learning.RLSOutput `json:"forecast,omitempty"`
	ForecastHorizon  int                 `json:"forecastHorizon" validate:"min=0"`
	CalibrationCount uint64              `json:"calibrationCount"`
	Confidence       float64             `json:"confidence" validate:"finite,min=0,max=1"`
	AvailableCapital *decimal.Decimal    `json:"availableCapital" validate:"required"`
	OpenPositions    int                 `json:"openPositions" validate:"min=0"`
	Cause            string              `json:"cause"`
	Reason           string              `json:"reason"`
	ReservationID    string              `json:"reservationId,omitempty"`
	SellableQty      *decimal.Decimal    `json:"sellableQty,omitempty"`
	EntryAt          *time.Time          `json:"entryAt,omitempty"`
	ExitAt           *time.Time          `json:"exitAt,omitempty"`
	EntryPrice       *decimal.Decimal    `json:"entryPrice,omitempty"`
	EntryFee         *decimal.Decimal    `json:"entryFee,omitempty"`
	ExitPrice        *decimal.Decimal    `json:"exitPrice,omitempty"`
	ExitFee          *decimal.Decimal    `json:"exitFee,omitempty"`
	PnL              *decimal.Decimal    `json:"pnl,omitempty"`
	ReturnPct        *float64            `json:"returnPct,omitempty"`
	Mark             *decimal.Decimal    `json:"mark,omitempty"`
	EntryCost        *EntryCost          `json:"entryCost,omitempty"`
	Stoploss         *Stoploss           `json:"stoploss,omitempty"`
	/*
		Risk is the stop geometry this entry was sized under. It travels with
		the decision because the quantity above was derived from it: the
		allocator capped the size so that the distance to the hard floor costs
		no more than the loss budget, and a stop later placed at some other
		distance would silently undo that.
	*/
	Risk RiskPlan `json:"risk"`

	// Trace is the reasoning record behind this decision: the War Room's
	// deliberation and the causal search it fed. It is present only for
	// rounds that actually ran a search.
	Trace *DecisionTrace
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

/*
StrategyRound carries one plan decision round intended for the dashboard. The
planner emits it as domain data on ChannelDecisions; the workspace observer
projects it into the StrategyFrame so the planner never publishes UI directly.
*/
type StrategyRound struct {
	Symbol    string
	Evaluated bool
	Outcome   string
	Decisions []*Decision
}
