package types

import "github.com/theapemachine/symm/market/perspectives"

/*
BestTree is an improved reasoning forest found during a search.
*/
type BestTree struct {
	Iteration int
	Score     float64
	Return    float64
	Trades    int
	Nodes     int
	Thoughts  []perspectives.Thought
}
