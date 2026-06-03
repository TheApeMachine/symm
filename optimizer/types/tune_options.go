package types

import "fmt"

/*
TuneOptions controls a measurement-backed optimizer run.
*/
type TuneOptions struct {
	OutputPath          string
	CandidateReportPath string
	MaxMeasurements     int
	Workers             int
	MaxThresholds       int
	BeamWidth           int
	CandidateLimit      int
	MaxReasoningSteps   int
	Hybrid              bool
	HybridSeedCount     int
	MCTSIterations      int
	Guard               GuardOptions
	OnBest              func(BestTree)
	OnCandidate         func(CandidateScore)
}

/*
SessionSummary is the optimizer output for one replay pass.
*/
type SessionSummary struct {
	MeasurementCount int     `json:"measurement_count"`
	BranchCount      int     `json:"branch_count"`
	Candidates       int     `json:"candidates"`
	Workers          int     `json:"workers"`
	HybridSeeds      int     `json:"hybrid_seeds,omitempty"`
	MCTSRounds       int     `json:"mcts_rounds,omitempty"`
	BestScore        float64 `json:"best_score"`
}

func (summary SessionSummary) String() string {
	if summary.MCTSRounds > 0 {
		return fmt.Sprintf(
			"measurements=%d branches=%d candidates=%d workers=%d seeds=%d mcts=%d score=%.6f",
			summary.MeasurementCount,
			summary.BranchCount,
			summary.Candidates,
			summary.Workers,
			summary.HybridSeeds,
			summary.MCTSRounds,
			summary.BestScore,
		)
	}

	if summary.Candidates > 0 {
		return fmt.Sprintf(
			"measurements=%d branches=%d candidates=%d workers=%d score=%.6f",
			summary.MeasurementCount,
			summary.BranchCount,
			summary.Candidates,
			summary.Workers,
			summary.BestScore,
		)
	}

	return fmt.Sprintf(
		"measurements=%d branches=%d score=%.6f",
		summary.MeasurementCount,
		summary.BranchCount,
		summary.BestScore,
	)
}
