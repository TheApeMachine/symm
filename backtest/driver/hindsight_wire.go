package driver

import (
	"sort"

	"github.com/theapemachine/symm/backtest/hindsight"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
publishHindsight emits one hindsight wire frame for the dashboard.
*/
func (driver *Driver) publishHindsight(report RealizedReport) {
	if driver.uiFeed != nil {
		driver.uiFeed.Emit(&types.UIFrame{
			Type:  wire.FrameHindsightFrame,
			Value: hindsightWire(report),
		})
	}
}

func hindsightWire(report RealizedReport) *wire.HindsightFrameT {
	symbols := make([]*wire.HindsightSymbolT, 0, len(report.Symbols))

	for _, symbol := range report.Symbols {
		opportunities := make([]*wire.HindsightOpportunityT, 0, len(symbol.Opportunities))

		for _, opportunity := range symbol.Opportunities {
			journal := make([]*wire.HindsightSignalT, 0, len(opportunity.Journal))

			for _, decision := range opportunity.Journal {
				journal = append(journal, hindsightDecisionSignalWire(decision))
			}

			opportunities = append(opportunities, &wire.HindsightOpportunityT{
				Leg:        hindsightLegWire(opportunity.Leg),
				Signal:     hindsightSignalWire(opportunity.Signal),
				Journal:    journal,
				Executable: hindsightExecutableWire(opportunity.Executable),
				Regret:     hindsightRegretWire(opportunity.Regret),
				Diagnosis:  hindsightDiagnosisWire(opportunity.Diagnosis),
				Why:        opportunity.Why,
				Captured:   opportunity.Captured,
				Missed:     opportunity.Missed,
			})
		}

		losses := make([]*wire.HindsightLossT, 0, len(symbol.Losses))

		for _, loss := range symbol.Losses {
			lossJournal := make([]*wire.HindsightSignalT, 0, len(loss.Journal))

			for _, decision := range loss.Journal {
				lossJournal = append(lossJournal, hindsightDecisionSignalWire(decision))
			}

			losses = append(losses, &wire.HindsightLossT{
				Symbol:        loss.Symbol,
				DecisionId:    loss.DecisionID,
				EntryAt:       loss.EntryAt.UnixNano(),
				ExitAt:        loss.ExitAt.UnixNano(),
				EntryPrice:    loss.EntryPrice,
				ExitPrice:     loss.ExitPrice,
				Pnl:           loss.LossPerUnit,
				ReturnPct:     loss.ReturnPct,
				GrossPct:      loss.GrossPct,
				FrictionPct:   loss.FrictionPct,
				TriggerReason: loss.TriggerReason,
				Diagnosis:     hindsightDiagnosisWire(loss.Diagnosis),
				Signal:        hindsightSignalWire(loss.Signal),
				Journal:       lossJournal,
			})
		}

		symbols = append(symbols, &wire.HindsightSymbolT{
			Symbol:                  symbol.Symbol,
			PriceTheoreticalCeiling: symbol.PriceTheoreticalCeiling,
			ExecutableCeiling:       symbol.ExecutableCeiling,
			ExecutableLegsDefined:   int64(symbol.ExecutableLegsDefined),
			ExecutableLegsTotal:     int64(symbol.ExecutableLegsTotal),
			RealizedPct:             symbol.RealizedPct,
			MissedPct:               symbol.MissedPct,
			LossPct:                 symbol.LossPct,
			Legs:                    int64(symbol.Legs),
			MissedLegs:              int64(symbol.MissedLegs),
			LossPositions:           int64(symbol.LossPositions),
			Opportunities:           opportunities,
			Losses:                  losses,
		})
	}

	rootCauses := hindsightRootCausesWire(report.RootCauses)

	recommendations := make([]*wire.HindsightRecommendationT, 0, len(report.Recommendations))

	for _, recommendation := range report.Recommendations {
		recommendations = append(
			recommendations,
			hindsightRecommendationWire(recommendation),
		)
	}

	lossRootCauses := hindsightRootCausesWire(report.LossRootCauses)

	lossRecommendations := make([]*wire.HindsightRecommendationT, 0, len(report.LossRecommendations))

	for _, recommendation := range report.LossRecommendations {
		lossRecommendations = append(
			lossRecommendations,
			hindsightRecommendationWire(recommendation),
		)
	}

	return &wire.HindsightFrameT{
		CaptureId:               report.CaptureID,
		Status:                  report.Status,
		Symbols:                 symbols,
		PriceTheoreticalCeiling: report.PriceTheoreticalCeiling,
		ExecutableCeiling:       report.ExecutableCeiling,
		MissedPct:               report.MissedPct,
		MissedLegs:              int64(report.MissedLegs),
		TotalLegs:               int64(report.TotalLegs),
		RealizedPct:             report.RealizedPct,
		LossPct:                 report.LossPct,
		LossPositions:           int64(report.LossPositions),
		ValueCaptureRate:        report.ValueCaptureRate,
		LegCaptureRate:          report.LegCaptureRate,
		DiagnosticCoverage:      report.DiagnosticCoverage,
		RootCauses:              rootCauses,
		Recommendations:         recommendations,
		LossRootCauses:          lossRootCauses,
		LossRecommendations:     lossRecommendations,
	}
}

func hindsightLegWire(leg hindsight.Leg) *wire.HindsightLegT {
	return &wire.HindsightLegT{
		Symbol:         leg.Symbol,
		BuyAt:          leg.BuyAt.UnixNano(),
		SellAt:         leg.SellAt.UnixNano(),
		BuyPrice:       leg.BuyPrice,
		SellPrice:      leg.SellPrice,
		ProfitPct:      leg.ProfitPct,
		GrossProfitPct: leg.GrossProfitPct,
		FrictionPct:    leg.FrictionPct,
	}
}

func hindsightExecutableWire(
	executable *hindsight.ExecutableLeg,
) *wire.HindsightExecutableT {
	if executable == nil {
		return nil
	}

	return &wire.HindsightExecutableT{
		Symbol:               executable.Symbol,
		BuyAt:                executable.BuyAt.UnixNano(),
		SellAt:               executable.SellAt.UnixNano(),
		TheoreticalBuyPrice:  executable.TheoreticalBuyPrice,
		TheoreticalSellPrice: executable.TheoreticalSellPrice,
		TheoreticalReturn:    executable.TheoreticalReturn,
		RequestedQty:         executable.RequestedQty,
		RequestedNotional:    executable.RequestedNotional,
		ExecutableEntryQty:   executable.ExecutableEntryQty,
		ExecutableEntryVwap:  executable.ExecutableEntryVWAP,
		ExecutableEntryValue: executable.ExecutableEntryValue,
		ExecutableEntryFees:  executable.ExecutableEntryFees,
		EntryImpact:          executable.EntryImpact,
		ExecutableExitQty:    executable.ExecutableExitQty,
		ExecutableExitVwap:   executable.ExecutableExitVWAP,
		ExecutableExitValue:  executable.ExecutableExitValue,
		ExecutableExitFees:   executable.ExecutableExitFees,
		ExitImpact:           executable.ExitImpact,
		FullyExecutable:      executable.FullyExecutable,
		ExecutableReturn:     executable.ExecutableReturn,
		ExecutablePnL:        executable.ExecutablePnL,
	}
}

func hindsightRegretWire(regret hindsight.RegretLayer) *wire.HindsightRegretT {
	return &wire.HindsightRegretT{
		Detection:  regret.Detection,
		Valuation:  regret.Valuation,
		Selection:  regret.Selection,
		Execution:  regret.Execution,
		Management: regret.Management,
	}
}

/*
hindsightDecisionSignalWire projects one journal decision onto the wire signal
shape.
*/
func hindsightDecisionSignalWire(decision hindsight.Decision) *wire.HindsightSignalT {
	return hindsightSignalWire(hindsight.SignalFromDecision(decision))
}

/*
hindsightSignalWire projects one current-architecture decision context onto the
wire signal shape. It no longer carries the retired Thesis/Graph scores.
*/
func hindsightSignalWire(signal hindsight.SignalContext) *wire.HindsightSignalT {
	return &wire.HindsightSignalT{
		Id:                       signal.ID,
		At:                       signal.At.UnixNano(),
		Action:                   signal.Action,
		Reason:                   signal.Reason,
		Cause:                    signal.Cause,
		Opportunity:              signal.Opportunity,
		OpportunityType:          signal.OpportunityType,
		OpportunityPhase:         signal.OpportunityPhase,
		ValuationAttempted:       signal.ValuationAttempted,
		ValuationAvailable:       signal.ValuationAvailable,
		ValuationStatus:          signal.ValuationStatus,
		CausalIdentification:     signal.CausalIdentification,
		CausalBlockingCoordinate: signal.CausalBlockingCoordinate,
		Utility:                  signal.Utility,
		UtilityAvailable:         signal.UtilityAvailable,
		ProposedQuantity:         signal.ProposedQuantity.Float(),
		ProposedNotional:         signal.ProposedNotional.Float(),
		AvailableCapital:         signal.AvailableCapital.Float(),
		AllocationClass:          signal.AllocationClass,
		AllocationHaircut:        signal.AllocationHaircut,
		ExpectedReturn:           signal.ExpectedReturn.Float(),
		ExpectedFees:             signal.ExpectedFees.Float(),
		ExpectedSpread:           signal.ExpectedSpread.Float(),
		ExpectedImpact:           signal.ExpectedImpact.Float(),
		AdverseSelection:         signal.AdverseSelection.Float(),
		Uncertainty:              signal.Uncertainty,
		OpenPositions:            int64(signal.OpenPositions),
		SlotCapacity:             int64(signal.SlotCapacity),
		EntryCost:                hindsightEntryCostWire(signal.EntryCost),
		Risk:                     hindsightRiskWire(signal.Risk),
		Mcts:                     hindsightMCTSWire(signal.MCTS),
		Alternatives:             hindsightNumbers(signal.Alternatives),
	}
}

func hindsightEntryCostWire(cost hindsight.EntryCost) *wire.HindsightEntryCostT {
	return &wire.HindsightEntryCostT{
		EntryPrice:    cost.EntryPrice.Float(),
		BestAsk:       cost.BestAsk.Float(),
		BestBid:       cost.BestBid.Float(),
		GrossNotional: cost.GrossNotional.Float(),
		EntryFee:      cost.EntryFee.Float(),
		Spread:        cost.Spread.Float(),
		Impact:        cost.Impact.Float(),
		BreakEven:     cost.BreakEven.Float(),
	}
}

func hindsightRiskWire(risk hindsight.RiskPlan) *wire.HindsightRiskT {
	return &wire.HindsightRiskT{
		Present:      risk.Present,
		RiskDistance: risk.RiskDistance.Float(),
		MaxLoss:      risk.MaxLoss.Float(),
		EntryFeeRate: risk.EntryFeeRate.Float(),
		ExitFeeRate:  risk.ExitFeeRate.Float(),
	}
}

func hindsightMCTSWire(mcts hindsight.DecisionTrace) *wire.HindsightMCTST {
	branches := make([]*wire.HindsightMCTSBranchT, 0, len(mcts.Branches))

	for _, branch := range mcts.Branches {
		branches = append(branches, &wire.HindsightMCTSBranchT{
			Action:     branch.Action,
			Visits:     int64(branch.Visits),
			MeanReward: branch.MeanReward,
		})
	}

	return &wire.HindsightMCTST{
		RecommendedAction: mcts.RecommendedAction,
		Iterations:        int64(mcts.Iterations),
		Branches:          branches,
	}
}

/*
hindsightRootCausesWire projects a root-cause summary list onto the wire shape.
*/
func hindsightRootCausesWire(
	causes []hindsight.RootCauseSummary,
) []*wire.HindsightRootCauseT {
	result := make([]*wire.HindsightRootCauseT, 0, len(causes))

	for _, cause := range causes {
		result = append(result, &wire.HindsightRootCauseT{
			Category:    cause.Category,
			ImpactPct:   cause.ImpactPct,
			Occurrences: int64(cause.Occurrences),
			Symbols:     cause.Symbols,
		})
	}

	return result
}

func hindsightDiagnosisWire(
	diagnosis hindsight.Diagnosis,
) *wire.HindsightDiagnosisT {
	if diagnosis.Category == "" && diagnosis.Summary == "" {
		return nil
	}

	blockers := make([]*wire.HindsightBlockerT, 0, len(diagnosis.Blockers))

	for _, blocker := range diagnosis.Blockers {
		blockers = append(blockers, &wire.HindsightBlockerT{
			Key:         blocker.Key,
			Category:    blocker.Category,
			Label:       blocker.Label,
			Source:      blocker.Source,
			Observed:    blocker.Observed,
			Target:      blocker.Target,
			HasTarget:   blocker.HasTarget,
			Gap:         blocker.Gap,
			Severity:    blocker.Severity,
			Explanation: blocker.Explanation,
		})
	}

	return &wire.HindsightDiagnosisT{
		Category:        diagnosis.Category,
		Summary:         diagnosis.Summary,
		EvidenceQuality: diagnosis.EvidenceQuality,
		EvidenceStatus:  diagnosis.EvidenceStatus,
		Blockers:        blockers,
		Recommendation:  hindsightRecommendationWire(diagnosis.Recommendation),
	}
}

func hindsightRecommendationWire(
	recommendation hindsight.Recommendation,
) *wire.HindsightRecommendationT {
	if recommendation.Key == "" {
		return nil
	}

	return &wire.HindsightRecommendationT{
		Key:          recommendation.Key,
		Kind:         recommendation.Kind,
		Target:       recommendation.Target,
		Title:        recommendation.Title,
		Action:       recommendation.Action,
		Rationale:    recommendation.Rationale,
		Current:      recommendation.Current,
		Suggested:    recommendation.Suggested,
		HasCurrent:   recommendation.HasCurrent,
		HasSuggested: recommendation.HasSuggested,
		Adjustment:   recommendation.Adjustment,
		Confidence:   recommendation.Confidence,
		ImpactPct:    recommendation.ImpactPct,
		Occurrences:  int64(recommendation.Occurrences),
		Symbols:      recommendation.Symbols,
	}
}

func hindsightNumbers(values map[string]float64) []*wire.NamedNumberT {
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
