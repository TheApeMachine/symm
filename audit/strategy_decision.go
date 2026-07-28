package audit

import (
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
StrategyDecision records one symbol-scoped strategy outcome with the friction,
capital, and slot evidence needed to explain why the planner held, entered,
rotated, or rejected that name on this tick.
*/
func StrategyDecision(
	recorder *Recorder,
	tick int64,
	lifecycle string,
	decision types.Decision,
) error {
	return Record(recorder, "strategy_decision", map[string]any{
		"tick":               tick,
		"symbol":             decision.Symbol,
		"lifecycle":          lifecycle,
		"action":             decision.Action,
		"cause":              decision.Cause,
		"reason":             decision.Reason,
		"utility":            decision.Utility,
		"executable_return":  executableReturn(decision),
		"expected_return":    decimalText(decision.ExpectedReturn),
		"expected_fees":      decimalText(decision.ExpectedFees),
		"expected_spread":    decimalText(decision.ExpectedSpread),
		"expected_impact":    decimalText(decision.ExpectedImpact),
		"adverse_selection":  decision.AdverseSelection,
		"uncertainty":        decision.Uncertainty,
		"confidence":         decision.Confidence,
		"opportunity_margin": decision.OpportunityMargin,
		"cognitive_lead":     decision.CognitiveLead,
		"basin_confidence":   decision.BasinConfidence,
		"allocation_haircut": decision.AllocationHaircut,
		"allocation_class":   decision.AllocationClass,
		"opportunity":        decision.Opportunity,
		"proposed_notional":  decimalText(decision.ProposedNotional),
		"proposed_quantity":  decimalText(decision.ProposedQuantity),
		"available_capital":  decimalText(decision.AvailableCapital),
		"open_positions":     decision.OpenPositions,
		"slot_capacity":      decision.SlotCapacity,
		"forecast_source":    decision.ForecastSource,
		"forecast_model":     decision.ForecastModel,
		"forecast_epoch":     decision.ForecastEpoch,
		"forecast_valid_to":  decision.ValidThroughEpoch,
		"calibration_count":  decision.CalibrationCount,
		"alternatives":       decision.Alternatives,
		"displaces":          decision.Displaces,
		"displaced_quantity": decimalText(decision.DisplacedQuantity),
		"displaced_price":    decimalText(decision.DisplacedPrice),
	})
}

func decimalText(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}

	return value.String()
}

func executableReturn(decision types.Decision) any {
	if decision.ExpectedReturn == nil || decision.ExpectedFees == nil ||
		decision.ExpectedSpread == nil || decision.ExpectedImpact == nil {
		return nil
	}

	value := decision.ExpectedReturn.Float64() -
		decision.ExpectedFees.Float64() -
		decision.ExpectedSpread.Float64() -
		decision.ExpectedImpact.Float64() -
		decision.AdverseSelection

	return strconv.FormatFloat(value, 'f', -1, 64)
}
