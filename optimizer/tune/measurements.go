package tune

import (
	"context"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/budget"
	"github.com/theapemachine/symm/optimizer/guard"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/mcts"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/scan"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
TuneMeasurements searches trees against a recorded measurement tape.
*/
func TuneMeasurements(
	ctx context.Context,
	rows []perspectives.Measurement,
	options types.TuneOptions,
) (types.SessionSummary, error) {
	log.TuneLog("building profile (%d rows)", len(rows))

	tuner := &Tuner{
		ctx:     ctx,
		profile: *profile.NewProfile(ctx),
	}

	for _, row := range rows {
		tuner.profile.Add(row)
	}

	tuner.profile.PrepareCache()

	log.TuneLog("precompiling replay tape")

	pool := qpool.NewQ(ctx, 1, options.Workers, qpool.NewConfig())
	defer pool.Close()

	tape := replay.PrecompileTape(rows)
	searchBudget := budget.DeriveSearchBudget(&tuner.profile, tape, options.Workers)

	budget.ApplyBudgetToTuneOptions(&options, searchBudget)

	log.TuneLog(
		"search budget: beam=%d candidates=%d thresholds=%d depth=%d mcts=%d",
		searchBudget.BeamWidth,
		searchBudget.CandidateLimit,
		searchBudget.MaxThresholds,
		searchBudget.MaxReasoningSteps,
		searchBudget.MCTSIterations,
	)

	reporter, err := io.NewCandidateReporter(options.CandidateReportPath)

	if err != nil {
		return types.SessionSummary{}, err
	}

	var writeErr error
	onBest := func(best types.BestTree) {
		if options.OutputPath != "" {
			if err := io.WriteBranches(options.OutputPath, best.Branches); err != nil {
				writeErr = err

				return
			}
		}

		if options.OnBest != nil {
			options.OnBest(best)
		}
	}
	onCandidate := func(candidate types.CandidateScore) {
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

	log.TuneLog("compiled %d decision ticks", tape.Len())

	scanOptions := types.ScanOptions{
		Workers:           options.Workers,
		MaxThresholds:     options.MaxThresholds,
		BeamWidth:         options.BeamWidth,
		CandidateLimit:    options.CandidateLimit,
		MaxReasoningSteps: options.MaxReasoningSteps,
		Guard:             options.Guard,
		Budget:            searchBudget,
		Pool:              pool,
	}

	var stats types.ScanStats
	var hybridStats mcts.HybridStats

	if options.Hybrid {
		tuner.branches, hybridStats, err = mcts.RunHybridSearch(
			ctx,
			&tuner.profile,
			rows,
			tape,
			mcts.HybridOptions{
				ScanOptions: scanOptions,
				MCTSOptions: mcts.Options{
					Iterations:        options.MCTSIterations,
					MaxReasoningSteps: options.MaxReasoningSteps,
					MaxThresholds:     options.MaxThresholds,
					Budget:            searchBudget,
				},
				SeedCount:   options.HybridSeedCount,
				Guard:       options.Guard,
				OnBest:      onBest,
				OnCandidate: onCandidate,
			},
		)

		if err != nil {
			return types.SessionSummary{}, err
		}

		stats = hybridStats.Scan
	} else {
		search := scan.NewScanSearchWithTape(ctx, &tuner.profile, rows, tape, scanOptions)
		search.OnBest = onBest
		search.OnCandidate = onCandidate

		tuner.branches, stats = search.Run()

		if options.Guard.WalkForward.Enabled {
			log.TuneLog("walk-forward validation")

			overfitGuard := guard.NewOverfitGuard(ctx, options.Guard, tape, &tuner.profile)
			tuner.branches = guard.SelectWalkForwardBest(
				overfitGuard,
				rows,
				search.WalkForwardFinalists(),
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
		return types.SessionSummary{}, writeErr
	}

	summary := tuner.Summary()
	summary.Candidates = stats.Candidates
	summary.Workers = stats.Workers
	summary.HybridSeeds = hybridStats.SeedCount
	summary.MCTSRounds = hybridStats.MCTSRounds

	return summary, nil
}
