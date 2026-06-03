package optimizer

import (
	"context"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
BestTree is one improved tree found during a search.
*/
type BestTree struct {
	Iteration int
	Score     float64
	Branches  perspectives.BranchList
}

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
	return reasoningDepth(candidate.Branches)
}

/*
TuneMeasurements searches trees against a recorded measurement tape.
*/
func TuneMeasurements(
	ctx context.Context,
	rows []perspectives.Measurement,
	options TuneOptions,
) (SessionSummary, error) {
	TuneLog("building profile (%d rows)", len(rows))

	tuner := &Tuner{
		ctx:     ctx,
		profile: Profile{ctx: ctx},
	}

	for _, row := range rows {
		tuner.profile.Add(row)
	}

	tuner.profile.PrepareCache()

	TuneLog("precompiling replay tape")

	pool := qpool.NewQ(ctx, 1, options.Workers, qpool.NewConfig())
	defer pool.Close()

	tape := PrecompileTape(rows)
	budget := DeriveSearchBudget(&tuner.profile, tape, options.Workers)

	applyBudgetToTuneOptions(&options, budget)

	TuneLog(
		"search budget: beam=%d candidates=%d thresholds=%d depth=%d mcts=%d",
		budget.BeamWidth,
		budget.CandidateLimit,
		budget.MaxThresholds,
		budget.MaxReasoningSteps,
		budget.MCTSIterations,
	)

	reporter, err := newCandidateReporter(options.CandidateReportPath)

	if err != nil {
		return SessionSummary{}, err
	}

	var writeErr error
	onBest := func(best BestTree) {
		if options.OutputPath != "" {
			if err := WriteBranches(options.OutputPath, best.Branches); err != nil {
				writeErr = err

				return
			}
		}

		if options.OnBest != nil {
			options.OnBest(best)
		}
	}
	onCandidate := func(candidate CandidateScore) {
		if options.OnCandidate != nil {
			options.OnCandidate(candidate)
		}

		if reporter == nil || writeErr != nil {
			return
		}

		if err := reporter.Write(candidate); err != nil {
			writeErr = err
		}
	}

	TuneLog("compiled %d decision ticks", tape.Len())

	scanOptions := ScanOptions{
		Workers:           options.Workers,
		MaxThresholds:     options.MaxThresholds,
		BeamWidth:         options.BeamWidth,
		CandidateLimit:    options.CandidateLimit,
		MaxReasoningSteps: options.MaxReasoningSteps,
		Guard:             options.Guard,
		Budget:            budget,
		Pool:              pool,
	}

	var stats ScanStats
	var hybridStats HybridStats

	if options.Hybrid {
		tuner.branches, hybridStats, err = RunHybridSearch(
			ctx,
			&tuner.profile,
			rows,
			tape,
			HybridOptions{
				ScanOptions: scanOptions,
				MCTSOptions: MCTSOptions{
					Iterations:        options.MCTSIterations,
					MaxReasoningSteps: options.MaxReasoningSteps,
					MaxThresholds:     options.MaxThresholds,
					Budget:            budget,
				},
				SeedCount:   options.HybridSeedCount,
				Guard:       options.Guard,
				OnBest:      onBest,
				OnCandidate: onCandidate,
			},
		)

		if err != nil {
			return SessionSummary{}, err
		}

		stats = hybridStats.Scan
	} else {
		search := NewScanSearchWithTape(ctx, &tuner.profile, rows, tape, scanOptions)
		search.onBest = onBest
		search.onCandidate = onCandidate

		tuner.branches, stats = search.Run()

		if options.Guard.WalkForward.Enabled {
			TuneLog("walk-forward validation")

			guard := NewOverfitGuard(ctx, options.Guard, tape, &tuner.profile)
			tuner.branches = SelectWalkForwardBest(
				guard,
				rows,
				search.walkForwardFinalists(),
				pool,
			)
		}
	}

	if reporter != nil {
		if err := reporter.Close(); err != nil && writeErr == nil {
			writeErr = err
		}
	}

	if writeErr != nil {
		return SessionSummary{}, writeErr
	}

	summary := tuner.Summary()
	summary.Candidates = stats.Candidates
	summary.Workers = stats.Workers
	summary.HybridSeeds = hybridStats.SeedCount
	summary.MCTSRounds = hybridStats.MCTSRounds

	return summary, nil
}

func countBranches(branches perspectives.BranchList) int {
	count := 0

	for _, branch := range branches {
		count++
		count += countBranches(perspectives.BranchList(branch.Branches))
	}

	return count
}
