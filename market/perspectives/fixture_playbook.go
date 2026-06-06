package perspectives

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
FixturePlaybook returns the deterministic in-memory playbook integration scenarios
use when market.perspectives.fixture_playbook is enabled.
*/
func FixturePlaybook() []reasoning.Thought {
	return []reasoning.Thought{
		{
			When: reasoning.Predicate{All: []reasoning.Predicate{
				{
					Subject:   reasoning.SubjectPosition,
					Op:        reasoning.ComparisonEquals,
					Lifecycle: types.ObservationNotHolding,
				},
				{
					Subject:  reasoning.SubjectSignal,
					Category: types.CategorySystemicSlump,
					Unit:     reasoning.UnitSNR,
					Op:       reasoning.ComparisonAtLeast,
					Value:    1.0,
				},
				{
					Subject:  reasoning.SubjectSignal,
					Category: types.CategoryVolumeStarvation,
					Unit:     reasoning.UnitSNR,
					Op:       reasoning.ComparisonAtLeast,
					Value:    1.0,
				},
			}},
			Do: reasoning.Act{Type: reasoning.ActionLimit},
		},
		{
			When: reasoning.Predicate{All: []reasoning.Predicate{
				{
					Subject:   reasoning.SubjectPosition,
					Op:        reasoning.ComparisonEquals,
					Lifecycle: types.ObservationHolding,
				},
				{
					Subject:  reasoning.SubjectSignal,
					Category: types.CategorySystemicBeta,
					Unit:     reasoning.UnitSNR,
					Op:       reasoning.ComparisonAtLeast,
					Value:    1.0,
				},
			}},
			Do: reasoning.Act{Type: reasoning.ActionSettlePosition},
		},
	}
}

/*
FixturePlaybookEntryMeasurements are explicit signal rows integration scenarios feed
the system to provoke an entry.
*/
func FixturePlaybookEntryMeasurements(symbol string, last float64) []types.Measurement {
	return []types.Measurement{
		{
			Symbol: symbol, Source: types.SourceSentiment, Category: types.CategorySystemicSlump,
			Strength: 0.8, Confidence: 0.6, SNR: 1.0, Last: last,
		},
		{
			Symbol: symbol, Source: types.SourcePumpDump, Category: types.CategoryVolumeStarvation,
			Strength: 0.8, Confidence: 0.6, SNR: 1.0, Last: last,
		},
	}
}

/*
FixturePlaybookExitMeasurements are explicit rows for provoking a holding exit.
*/
func FixturePlaybookExitMeasurements(symbol string, last float64) []types.Measurement {
	return []types.Measurement{
		{
			Symbol: symbol, Source: types.SourceCorrelation, Category: types.CategorySystemicBeta,
			Strength: 0.8, Confidence: 0.6, SNR: 1.5, Last: last,
		},
	}
}
