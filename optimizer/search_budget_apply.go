package optimizer

import (
	"runtime"
)

func applyBudgetToTuneOptions(options *TuneOptions, budget SearchBudget) {
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

func applyBudgetToScanOptions(options ScanOptions, budget SearchBudget) ScanOptions {
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

func applyBudgetToMCTSOptions(options MCTSOptions, budget SearchBudget) MCTSOptions {
	if options.Iterations <= 0 {
		options.Iterations = budget.MCTSIterations
	}

	if options.SeedPriorVisits <= 0 {
		options.SeedPriorVisits = budget.MCTSSeedPriorVisits
	}

	if options.MaxReasoningSteps <= 0 {
		options.MaxReasoningSteps = budget.MaxReasoningSteps
	}

	if options.MaxThresholds <= 0 {
		options.MaxThresholds = budget.MaxThresholds
	}

	return options
}
