package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/tune"
	"github.com/theapemachine/symm/optimizer/types"
)

type (
	BestTree       = types.BestTree
	CandidateScore = types.CandidateScore
	GuardOptions   = types.GuardOptions
	ScanOptions    = types.ScanOptions
	ScanStats      = types.ScanStats
	SearchBudget   = types.SearchBudget
	SessionSummary = types.SessionSummary
	TuneOptions    = types.TuneOptions
)

var (
	TuneLog                    = log.TuneLog
	DefaultTuneOptions         = budget.DefaultTuneOptions
	DeriveMeasurementSampleCap = budget.DeriveMeasurementSampleCap
	CountMeasurementLines      = io.CountMeasurementLines
	LoadMeasurements           = io.LoadMeasurements
	TuneMeasurements           = tune.TuneMeasurements
)

func WriteBranches(path string, branches perspectives.BranchList) error {
	return io.WriteBranches(path, branches)
}
