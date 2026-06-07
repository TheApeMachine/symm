package types

import (
	"fmt"

	"github.com/theapemachine/symm/broker"
)

/*
TuneOptions controls a measurement-backed optimizer run. Zero search fields fall
back to the reasoning search's own defaults.
*/
type TuneOptions struct {
	OutputPath          string
	CandidateReportPath string
	MaxMeasurements     int
	Workers             int
	BeamWidth           int
	MaxRounds           int
	MaxNodes            int
	MinRoundTrips       int
	InstrumentRules     *broker.InstrumentRulesCache
	OnBest              func(BestTree)
	OnCandidate         func(CandidateScore)
}

/*
SessionSummary is the optimizer output for one search.
*/
type SessionSummary struct {
	MeasurementCount     int     `json:"measurement_count"`
	FundableMeasurements int     `json:"fundable_measurements"`
	MinRoundTrips        int     `json:"min_round_trips"`
	Strategies           int     `json:"strategies"`
	Nodes                int     `json:"nodes"`
	Trades               int     `json:"trades"`
	Evaluated            int     `json:"evaluated"`
	BestReturn           float64 `json:"best_return"`
	BestScore            float64 `json:"best_score"`
}

func (summary SessionSummary) String() string {
	return fmt.Sprintf(
		"measurements=%d fundable_measurements=%d min_round_trips=%d strategies=%d nodes=%d trades=%d evaluated=%d return=%.6f score=%.6f",
		summary.MeasurementCount,
		summary.FundableMeasurements,
		summary.MinRoundTrips,
		summary.Strategies,
		summary.Nodes,
		summary.Trades,
		summary.Evaluated,
		summary.BestReturn,
		summary.BestScore,
	)
}
