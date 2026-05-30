package perspectives

/*
Perspective is one trade thesis encoded as entry and exit decision trees.
*/
type Perspective interface {
	Name() PlaybookName
	Walk(measurements []Measurement) Perspective
	Decide(measurements []Measurement, observations []ObservationType) *ActionType
	DecideExit(measurements []Measurement, observations []ObservationType) *ActionType
	Regime() Regime
}
