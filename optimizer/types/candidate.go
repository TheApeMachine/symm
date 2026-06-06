package types

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
CandidateScore is one scored reasoning forest emitted by the search.
*/
type CandidateScore struct {
	Candidate    int
	Score        float64
	ClosedTrades int
	Depth        int
	Strategies   int
	Thoughts     []reasoning.Thought
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

// ReasoningDepth is the deepest Then-chain in the forest (its temporal depth).
func (candidate CandidateScore) ReasoningDepth() int {
	return candidate.Depth
}

// RegistryWidth is the number of parallel strategy branches.
func (candidate CandidateScore) RegistryWidth() int {
	return candidate.Strategies
}
