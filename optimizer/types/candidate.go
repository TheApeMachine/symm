package types

import (
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
CandidateScore is one scored reasoning forest emitted by the search.
*/
type CandidateScore struct {
	Candidate       int
	Score           float64
	ReturnFraction  float64
	RealizedEUR     float64
	ClosedTrades    int
	Depth           int
	Strategies      int
	Thoughts        []reasoning.Thought
	StartingCapital float64
}

func (candidate CandidateScore) ProfitLoss() float64 {
	return candidate.RealizedEUR
}

func (candidate CandidateScore) ReturnPerTrade() float64 {
	if candidate.ClosedTrades <= 0 {
		return 0
	}

	return candidate.ReturnFraction / float64(candidate.ClosedTrades)
}

func (candidate CandidateScore) ReturnPct() float64 {
	if candidate.StartingCapital <= 0 {
		return 0
	}

	return (candidate.RealizedEUR / candidate.StartingCapital) * 100
}

func (candidate CandidateScore) AvgTradeEUR() float64 {
	if candidate.ClosedTrades <= 0 {
		return 0
	}

	return candidate.RealizedEUR / float64(candidate.ClosedTrades)
}

// ReasoningDepth is the deepest Then-chain in the forest (its temporal depth).
func (candidate CandidateScore) ReasoningDepth() int {
	return candidate.Depth
}

// RegistryWidth is the number of parallel strategy branches.
func (candidate CandidateScore) RegistryWidth() int {
	return candidate.Strategies
}
