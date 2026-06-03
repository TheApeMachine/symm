package types

import "github.com/theapemachine/symm/market/perspectives"

/*
BestTree is one improved tree found during a search.
*/
type BestTree struct {
	Iteration int
	Score     float64
	Branches  perspectives.BranchList
}
