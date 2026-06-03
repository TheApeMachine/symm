package budget

import (
	"runtime"

	"github.com/theapemachine/symm/optimizer/types"
)

func ApplyBudgetToTuneOptions(options *types.TuneOptions, budget types.SearchBudget) {
	applyBudgetToTuneOptions(options, budget)
}

func applyBudgetToTuneOptions(options *types.TuneOptions, budget types.SearchBudget) {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	if options.MaxMeasurements <= 0 {
		options.MaxMeasurements = budget.MeasurementSampleCap
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	if options.BeamWidth <= 0 {
		options.BeamWidth = budget.BeamWidth
	}

	if options.CandidateLimit <= 0 {
		options.CandidateLimit = budget.CandidateLimit
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.HybridSeedCount <= 0 {
		options.HybridSeedCount = budget.HybridSeedCount
	}

	if options.MCTSIterations <= 0 {
		options.MCTSIterations = budget.MCTSIterations
	}

	if options.Guard.MaxReasoningSteps <= 0 {
		options.Guard.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MinRoundTrips <= 0 {
		options.Guard.MinRoundTrips = budget.MinRoundTrips
	}

	options.Guard.ComplexityPenalty = budget.ComplexityPenalty
}

func ApplyBudgetToScanOptions(
	options types.ScanOptions, budget types.SearchBudget,
) types.ScanOptions {
	return applyBudgetToScanOptions(options, budget)
}

func applyBudgetToScanOptions(
	options types.ScanOptions, budget types.SearchBudget,
) types.ScanOptions {
	if options.Workers <= 0 {
		options.Workers = runtime.NumCPU()
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	if options.BeamWidth <= 0 {
		options.BeamWidth = budget.BeamWidth
	}

	if options.CandidateLimit <= 0 {
		options.CandidateLimit = budget.CandidateLimit
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MaxReasoningSteps <= 0 {
		options.Guard.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.Guard.MinRoundTrips <= 0 {
		options.Guard.MinRoundTrips = budget.MinRoundTrips
	}

	options.Guard.ComplexityPenalty = budget.ComplexityPenalty
	options.Budget = budget

	return options
}
