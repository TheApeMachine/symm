package main

import (
	"strings"

	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
buildRound projects one persisted Decision onto the shared record.

The alternatives map is the planner's own key/value record of the round, and
its keys are already namespaced by concern (move:, consensus:, execution:,
branch:, search:). Splitting them back out here keeps the exported object
readable without inventing any value the planner did not write.
*/
func buildRound(
	runID string,
	sequence uint64,
	ordinal uint64,
	state *telemetry.EnvelopeState,
	decision *telemetry.Decision,
) round {
	record := round{
		Run:              runID,
		Sequence:         sequence,
		Ordinal:          ordinal,
		Tick:             state.Tick(),
		Symbol:           string(decision.Symbol()),
		Action:           string(decision.Action()),
		PredictiveStatus: string(decision.PredictiveStatus()),
		PredictiveReady:  decision.PredictiveReady(),
		Reason:           string(decision.Reason()),
		Cause:            string(decision.Cause()),
		Confidence:       decision.Confidence(),
		ForecastSource:   string(decision.ForecastSource()),
		ForecastModel:    string(decision.ForecastModel()),
		ForecastHorizon:  decision.ForecastHorizon(),
		CalibrationCount: decision.CalibrationCount(),
		ReferencePrice:   string(decision.ReferencePrice()),
		ProposedNotional: string(decision.ProposedNotional()),
		TaskSkill:        decision.TaskSkill(),
	}

	for index := range decision.AlternativesLength() {
		alternative := new(telemetry.NamedNumber)

		if !decision.Alternatives(alternative, index) {
			continue
		}

		name := string(alternative.Name())
		value := alternative.Value()

		switch {
		case strings.HasPrefix(name, "move:"):
			record.Probabilities = put(record.Probabilities,
				strings.TrimPrefix(name, "move:"), value)
		case strings.HasPrefix(name, "consensus:"):
			record.Consensus = put(record.Consensus,
				strings.TrimPrefix(name, "consensus:"), value)
		case strings.HasPrefix(name, "execution:"):
			record.Execution = put(record.Execution,
				strings.TrimPrefix(name, "execution:"), value)
		}
	}

	record.Search = buildSearch(decision.Trace(nil))

	return record
}

func put(target map[string]float64, key string, value float64) map[string]float64 {
	if target == nil {
		target = make(map[string]float64)
	}

	target[key] = value

	return target
}

/*
buildSearch projects the causal search's trace. A round that stopped at an
earlier gate has no trace, and reports none rather than an empty search that
would read as a search that found nothing.
*/
func buildSearch(trace *telemetry.DecisionTrace) *searchRound {
	if trace == nil {
		return nil
	}

	search := &searchRound{
		RecommendedAction:    string(trace.RecommendedAction()),
		IdentificationStatus: string(trace.IdentificationStatus()),
		DecisionUnavailable:  trace.DecisionUnavailable(),
		Iterations:           trace.Iterations(),
		Horizon:              trace.Horizon(),
		MaxDepth:             trace.MaxDepth(),
		TotalNodes:           trace.TotalNodes(),
		ExpectedOutcome:      trace.ExpectedOutcome(),
		OutcomeUncertainty:   trace.OutcomeUncertainty(),
		TransitionSource:     string(trace.TransitionSource()),
		DominantMove:         string(trace.ConsensusDominantMove()),
		Participants:         trace.ConsensusParticipants(),
	}

	for index := range trace.VetoesLength() {
		search.Vetoes = append(search.Vetoes, string(trace.Vetoes(index)))
	}

	for index := range trace.SynergiesLength() {
		search.Synergies = append(search.Synergies, string(trace.Synergies(index)))
	}

	for index := range trace.BranchesLength() {
		branch := new(telemetry.MCTSBranch)

		if !trace.Branches(branch, index) {
			continue
		}

		search.Branches = append(search.Branches, searchBranch{
			Action:             string(branch.Action()),
			Visits:             branch.Visits(),
			MeanReward:         branch.MeanReward(),
			BlendedValue:       branch.BlendedValue(),
			RewardStd:          branch.RewardStd(),
			CounterfactualMass: branch.CounterfactualMass(),
			CausalExpectation:  branch.CausalExpectation(),
			CausalDefined:      branch.CausalExpectationDefined(),
			Pruned:             branch.Pruned(),
		})
	}

	return search
}
