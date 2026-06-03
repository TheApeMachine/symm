package types

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/playbook"
)

/*
CandidateScore is one scored candidate tree emitted by the scanner.
*/
type CandidateScore struct {
	Candidate     int
	Score         float64
	AdjustedScore float64
	ClosedTrades  int
	Branches      perspectives.BranchList
}

func (candidate CandidateScore) ProfitLoss() float64 {
	return candidate.Score
}

func (candidate CandidateScore) ReturnPerTrade() float64 {
	if candidate.ClosedTrades <= 0 {
		return 0
	}

	return candidate.Score / float64(candidate.ClosedTrades)
}

func (candidate CandidateScore) ReturnPct() float64 {
	return candidate.ReturnPerTrade() * 100
}

func (candidate CandidateScore) BranchCount() int {
	return countBranches(candidate.Branches)
}

func (candidate CandidateScore) RegistryWidth() int {
	return len(candidate.Branches)
}

func (candidate CandidateScore) ReasoningDepth() int {
	return playbook.ReasoningDepth(candidate.Branches)
}

func countBranches(branches perspectives.BranchList) int {
	count := 0

	for _, branch := range branches {
		count++
		count += countBranches(perspectives.BranchList(branch.Branches))
	}

	return count
}
