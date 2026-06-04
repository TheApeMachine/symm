package perspectives

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
