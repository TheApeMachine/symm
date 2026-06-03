package perspectives

/*
FixturePlaybookBranches is the stable branch registry for unit and integration tests.

Production Story loads cfg/perspectives.yaml, which the optimizer rewrites. Tests must
use this fixture (via market.perspectives.fixture_playbook) and explicit measurement
rows — not the embedded file or EntryPassMeasurements against it.
*/
func FixturePlaybookBranches() BranchList {
	return BranchList{
		{
			Category:  CategorySystemicSlump,
			Condition: ConditionIsGreaterThanOrEqual,
			Unit:      UnitSNR,
			Value:     0,
			ValueSet:  true,
			Branches: BranchList{{
				Category:    CategoryVolumeStarvation,
				Observation: ObservationNotHolding,
				Condition:   ConditionIsGreaterThanOrEqual,
				Unit:        UnitSNR,
				Value:       0,
				ValueSet:    true,
				Action:      Action{Type: ActionLimit},
			}},
		},
		{
			Category:    CategorySystemicBeta,
			Observation: ObservationHolding,
			Condition:   ConditionIsGreaterThanOrEqual,
			Unit:        UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      Action{Type: ActionSettlePosition},
		},
	}
}

/*
FixturePlaybookEntryMeasurements are explicit rows that satisfy FixturePlaybookBranches.
*/
func FixturePlaybookEntryMeasurements(symbol string, last float64) []Measurement {
	return []Measurement{
		{Symbol: symbol, Category: CategorySystemicSlump, SNR: 1.0, Last: last},
		{Symbol: symbol, Category: CategoryVolumeStarvation, SNR: 1.0, Last: last},
	}
}

/*
FixturePlaybookExitMeasurements are explicit rows for the fixture holding exit branch.
*/
func FixturePlaybookExitMeasurements(symbol string, last float64) []Measurement {
	return []Measurement{
		{Symbol: symbol, Category: CategorySystemicBeta, SNR: 1.5, Last: last},
	}
}
