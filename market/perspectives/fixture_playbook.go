package perspectives

/*
FixturePlaybook returns the deterministic in-memory playbook integration scenarios
use when market.perspectives.fixture_playbook is enabled.
*/
func FixturePlaybook() []Thought {
	return []Thought{
		{
			When: Predicate{All: []Predicate{
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: ObservationNotHolding,
				},
				{
					Subject:  SubjectSignal,
					Category: CategorySystemicSlump,
					Unit:     UnitSNR,
					Op:       ComparisonAtLeast,
					Value:    1.0,
				},
				{
					Subject:  SubjectSignal,
					Category: CategoryVolumeStarvation,
					Unit:     UnitSNR,
					Op:       ComparisonAtLeast,
					Value:    1.0,
				},
			}},
			Do: Act{Type: ActionLimit},
		},
		{
			When: Predicate{All: []Predicate{
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: ObservationHolding,
				},
				{
					Subject:  SubjectSignal,
					Category: CategorySystemicBeta,
					Unit:     UnitSNR,
					Op:       ComparisonAtLeast,
					Value:    1.0,
				},
			}},
			Do: Act{Type: ActionSettlePosition},
		},
	}
}

/*
FixturePlaybookEntryMeasurements are explicit signal rows integration scenarios feed
the system to provoke an entry.
*/
func FixturePlaybookEntryMeasurements(symbol string, last float64) []Measurement {
	return []Measurement{
		{Symbol: symbol, Category: CategorySystemicSlump, SNR: 1.0, Last: last},
		{Symbol: symbol, Category: CategoryVolumeStarvation, SNR: 1.0, Last: last},
	}
}

/*
FixturePlaybookExitMeasurements are explicit rows for provoking a holding exit.
*/
func FixturePlaybookExitMeasurements(symbol string, last float64) []Measurement {
	return []Measurement{
		{Symbol: symbol, Category: CategorySystemicBeta, SNR: 1.5, Last: last},
	}
}
