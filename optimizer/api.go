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

// WriteThoughts persists a reasoning forest to a playbook file.
func WriteThoughts(path string, thoughts []perspectives.Thought) error {
	return io.WriteThoughts(path, thoughts)
}
