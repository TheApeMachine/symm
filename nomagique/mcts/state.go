package mcts

/*
State is the compatibility boundary used by the search engine. Implementations
must describe market interventions rather than graph-navigation choices.
*/
type State interface {
	GetPossibleActions() []float64
	ApplyAction(action float64) State
	IsTerminal() bool
	GetReward() float64
	ToVector() []float64
}
