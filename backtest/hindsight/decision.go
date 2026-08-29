package hindsight

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

/*
Number decodes a numeric field that the audit stream may have serialized as a
JSON number or as a fixed-point string (types.Decimal marshals to a string). It
is float64-backed because hindsight only ever needs the comparison and notional
arithmetic a float supports, and it keeps the decision-window decoder honest
about both shapes.
*/
type Number float64

/*
UnmarshalJSON accepts either a JSON number or a JSON string encoding a number.
*/
func (number *Number) UnmarshalJSON(data []byte) error {
	if number == nil {
		return nil
	}

	if len(data) > 0 && data[0] == '"' {
		var text string

		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}

		if text == "" {
			*number = 0
			return nil
		}

		parsed, err := strconv.ParseFloat(text, 64)

		if err != nil {
			return err
		}

		*number = Number(parsed)
		return nil
	}

	var parsed float64

	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}

	*number = Number(parsed)

	return nil
}

/*
Float returns the numeric value as a float64.
*/
func (number Number) Float() float64 {
	return float64(number)
}

/*
Decision is the window into the audit record that hindsight needs: what the
system decided, when, and the valuation/allocation/execution state it recorded.
Only these fields are decoded so this remains the full coupling between
hindsight and the decision stream; the heavy types package is not imported.
*/
type Decision struct {
	ID     string    `json:"id"`
	Action string    `json:"action"`
	Symbol string    `json:"symbol"`
	At     time.Time `json:"at"`

	// Valuation state — the Opportunity → Valuation boundary.
	ValuationAttempted      bool   `json:"valuationAttempted"`
	ValuationAvailable      bool   `json:"valuationAvailable"`
	ValuationStatus         string `json:"valuationStatus,omitempty"`
	CausalIdentification    string `json:"causalIdentification,omitempty"`
	CausalBlockingCoordinate string `json:"causalBlockingCoordinate,omitempty"`

	// Selection state — what MCTS compared and chose.
	Utility          float64             `json:"utility"`
	UtilityAvailable bool                `json:"utilityAvailable"`
	Alternatives     map[string]float64 `json:"alternatives"`
	Opportunity      bool                `json:"opportunity"`
	OpportunityType  string              `json:"opportunityType,omitempty"`
	OpportunityPhase string              `json:"opportunityPhase,omitempty"`
	Trace            DecisionTrace       `json:"trace,omitempty"`

	// Execution state — the size and risk this entry was solved under.
	ProposedQuantity Number   `json:"proposedQuantity"`
	ProposedNotional Number   `json:"proposedNotional"`
	AvailableCapital Number   `json:"availableCapital"`
	EntryCost        EntryCost `json:"entryCost,omitempty"`
	Risk             RiskPlan  `json:"risk"`
	ExpectedReturn   Number   `json:"expectedReturn"`
	ExpectedFees     Number   `json:"expectedFees"`
	ExpectedSpread   Number   `json:"expectedSpread"`
	ExpectedImpact   Number   `json:"expectedImpact"`
	AdverseSelection Number   `json:"adverseSelection"`
	Uncertainty      float64  `json:"uncertainty"`

	// Allocation state.
	AllocationClass   string `json:"allocationClass"`
	AllocationHaircut float64 `json:"allocation_haircut"`
	OpenPositions     int    `json:"openPositions"`
	SlotCapacity      int    `json:"slotCapacity"`

	// Narrative fallback fields retained for diagnosis prose.
	Cause  string `json:"cause"`
	Reason string `json:"reason"`
}

/*
DecisionTrace projects the MCTS entry-decision chain: the recommended action and
the branches actually explored, straight off the search tree.
*/
type DecisionTrace struct {
	RecommendedAction string              `json:"recommendedAction,omitempty"`
	Iterations        int                 `json:"iterations"`
	Branches          []DecisionMCTSBranch `json:"branches"`
}

/*
DecisionMCTSBranch is one root child actually explored by the causal search.
*/
type DecisionMCTSBranch struct {
	Action     string  `json:"action"`
	Visits     int     `json:"visits"`
	MeanReward float64 `json:"meanReward"`
}

/*
EntryCost mirrors the observable execution boundary one long was priced at:
entry VWAP, fees, spread, impact, and break-even.
*/
type EntryCost struct {
	EntryPrice   Number `json:"entryPrice"`
	BestAsk      Number `json:"bestAsk"`
	BestBid      Number `json:"bestBid"`
	GrossNotional Number `json:"grossNotional"`
	EntryFee     Number `json:"entryFee"`
	RoundTripFees Number `json:"roundTripFees"`
	Spread       Number `json:"spread"`
	Impact       Number `json:"impact"`
	BreakEven    Number `json:"breakEven"`
}

/*
RiskPlan carries the stop geometry and fee rates one lot was sized under.
*/
type RiskPlan struct {
	Present      bool   `json:"present"`
	RiskDistance Number `json:"risk_distance"`
	MaxLoss      Number `json:"max_loss"`
	EntryFeeRate Number `json:"entry_fee_rate"`
	ExitFeeRate  Number `json:"exit_fee_rate"`
}

/*
decodeDecisions unmarshals one raw audit payload into hindsight decisions.
*/
func decodeDecisions(payload []byte) ([]Decision, error) {
	var decisions []Decision

	if err := json.Unmarshal(payload, &decisions); err != nil {
		return nil, fmt.Errorf("hindsight: decode decisions: %w", err)
	}

	return decisions, nil
}
