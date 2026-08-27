package types

import (
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/nomagique/mcts"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func DecisionWire(
	decision Decision,
	branchLimit int,
	includeTree bool,
) *wire.DecisionT {
	alternatives := namedNumbers(decision.Alternatives)

	return &wire.DecisionT{
		Id:                        decision.ID,
		Action:                    string(decision.Action),
		Symbol:                    decision.Symbol,
		At:                        timeNano(decision.At),
		Utility:                   decision.Utility,
		UtilityAvailable:          decision.UtilityAvailable,
		ValuationAttempted:        decision.ValuationAttempted,
		ValuationAvailable:        decision.ValuationAvailable,
		ValuationStatus:           decision.ValuationStatus,
		CausalIdentification:     decision.CausalIdentification,
		CausalBlockingCoordinate:  decision.CausalBlockingCoordinate,
		GraphScore:                decision.GraphScore,
		ThesisScore:               decision.ThesisScore,
		ThesisConfidence:          decision.ThesisConfidence,
		ThesisSupport:             decision.ThesisSupport,
		ThesisContradiction:       decision.ThesisContradiction,
		ThesisConditions:          decision.ThesisConditions,
		Direction:                 decision.Direction,
		PerspectiveReturn:         decision.PerspectiveReturn,
		PerspectiveConfidence:     decision.PerspectiveConfidence,
		AdmissionGraphThreshold:   decision.AdmissionGraphThreshold,
		AdmissionUtilityThreshold: decision.AdmissionUtilityThreshold,
		AllocationHaircut:         decision.AllocationHaircut,
		AllocationHaircutReason:   decision.AllocationHaircutReason,
		Alternatives:              alternatives,
		AllocationClass:           decision.AllocationClass,
		Opportunity:               decision.Opportunity,
		OpportunityType:           string(decision.OpportunityType),
		OpportunityPhase:          string(decision.OpportunityPhase),
		ReserveEligible:           decision.ReserveEligible,
		ReserveReason:             decision.ReserveReason,
		PredictiveReady:           decision.PredictiveReady,
		PredictiveStatus:          decision.PredictiveStatus,
		TaskSkill:                 decision.TaskSkill,
		TaskSkillReady:            decision.TaskSkillReady,
		ProposedNotional:          decimalString(decision.ProposedNotional),
		ProposedQuantity:          decimalString(decision.ProposedQuantity),
		ReferencePrice:            decimalString(decision.ReferencePrice),
		ValidThroughEpoch:         decision.ValidThroughEpoch,
		ArbitrationRound:          decision.ArbitrationRound,
		ForecastSource:            decision.ForecastSource,
		ForecastModel:             decision.ForecastModel,
		ForecastEpoch:             decision.ForecastEpoch,
		ForecastHorizon:           int64(decision.ForecastHorizon),
		ForwardCurve:              decision.ForwardCurve,
		CalibrationCount:          decision.CalibrationCount,
		ExpectedReturn:            decimalString(decision.ExpectedReturn),
		ExpectedFees:              decimalString(decision.ExpectedFees),
		ExpectedSpread:            decimalString(decision.ExpectedSpread),
		ExpectedImpact:            decimalString(decision.ExpectedImpact),
		AdverseSelection:          decimalString(decision.AdverseSelection),
		Uncertainty:               decision.Uncertainty,
		Confidence:                decision.Confidence,
		CausalPrecision:           decision.CausalPrecision,
		OpportunityMargin:         decision.OpportunityMargin,
		CognitiveLead:             decision.CognitiveLead,
		BasinConfidence:           decision.BasinConfidence,
		AvailableCapital:          decimalString(decision.AvailableCapital),
		OpenPositions:             int64(decision.OpenPositions),
		SlotCapacity:              int64(decision.SlotCapacity),
		Cause:                     decision.Cause,
		Reason:                    decision.Reason,
		Displaces:                 decision.Displaces,
		DisplacedQuantity:         decimalString(decision.DisplacedQuantity),
		DisplacedPrice:            decimalString(decision.DisplacedPrice),
		ReservationId:             decision.ReservationID,
		PositionStatus:            string(decision.PositionStatus),
		SellableQty:               decimalString(decision.SellableQty),
		EntryAt:                   timePointerNano(decision.EntryAt),
		ExitAt:                    timePointerNano(decision.ExitAt),
		EntryPrice:                decimalString(decision.EntryPrice),
		EntryFee:                  decimalString(decision.EntryFee),
		ExitPrice:                 decimalString(decision.ExitPrice),
		ExitFee:                   decimalString(decision.ExitFee),
		Pnl:                       decimalString(decision.PnL),
		ReturnPct:                 floatPointer(decision.ReturnPct),
		Mark:                      decimalString(decision.Mark),
		EntryCost:                 entryCostWire(decision.EntryCost),
		Stoploss:                  StoplossWire(decision.Stoploss),
		Risk:                      riskWire(&decision.Risk),
		Trace: decisionTraceWire(
			decision.Trace,
			branchLimit,
			includeTree,
		),
	}
}

func namedNumbers(values map[string]float64) []*wire.NamedNumberT {
	names := make([]string, 0, len(values))

	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)
	result := make([]*wire.NamedNumberT, 0, len(names))

	for _, name := range names {
		result = append(result, &wire.NamedNumberT{Name: name, Value: values[name]})
	}

	return result
}

func decisionTraceWire(
	trace *DecisionTrace,
	branchLimit int,
	includeTree bool,
) *wire.DecisionTraceT {
	if trace == nil {
		return nil
	}

	branchCount := min(len(trace.MCTS.Branches), max(branchLimit, 0))
	branches := make([]*wire.MCTSBranchT, 0, branchCount)

	for _, branch := range trace.MCTS.Branches[:branchCount] {
		branches = append(branches, &wire.MCTSBranchT{
			Action: branch.Action, Visits: int64(branch.Visits), MeanReward: branch.MeanReward,
		})
	}

	encoded := &wire.DecisionTraceT{
		Hypothesis:        trace.Hypothesis,
		GraphSupports:     trace.GraphSupports,
		GraphContradicts:  trace.GraphContradicts,
		GraphConditions:   trace.GraphConditions,
		ThesisBalance:     trace.ThesisBalance,
		ThesisConfidence:  trace.ThesisConfidence,
		Iterations:        int64(trace.MCTS.Iterations),
		Branches:          branches,
		RecommendedAction: trace.MCTS.RecommendedAction,
	}

	if includeTree {
		encoded.Tree = mctsNodeWire(trace.MCTS.Tree)
	}

	return encoded
}

func mctsNodeWire(node *mcts.SearchNode) *wire.MCTSNodeT {
	if node == nil {
		return nil
	}

	children := make([]*wire.MCTSNodeT, 0, len(node.Children))

	for _, child := range node.Children {
		children = append(children, mctsNodeWire(child))
	}

	return &wire.MCTSNodeT{
		Action:     int64(node.Action),
		ActionName: node.Action.String(),
		Depth:      int64(node.Depth),
		Visits:     int64(node.Visits),
		MeanReward: node.MeanReward(),
		Children:   children,
	}
}

func decimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}

	return value.String()
}

func floatPointer(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

func timeNano(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return at.UnixNano()
}

func timePointerNano(at *time.Time) int64 {
	if at == nil {
		return 0
	}

	return timeNano(*at)
}
