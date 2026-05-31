package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/config"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

type tuneRunOptions struct {
	replayFile         string
	holdoutFiles       []string
	autoHoldout        bool
	walkForwardFolds   int
	perturbTrain       bool
	stressHoldout      bool
	maxTrials          int
	workers            int
	evalWorkers        int
	output             string
	perspectiveOutput  string
	maxTrainHoldoutGap float64
	quiet              bool
}

type tuneRunResult struct {
	payload []byte
}

func runTune(parent context.Context, cmd *cobra.Command) (tuneRunResult, error) {
	options, err := tuneOptionsFromCommand(cmd)

	if err != nil {
		return tuneRunResult{}, err
	}

	replayPaths, err := resolveTuneReplayPaths(
		options.replayFile,
		options.holdoutFiles,
		options.autoHoldout,
		options.walkForwardFolds,
	)

	if err != nil {
		return tuneRunResult{}, err
	}

	if replayPaths.cleanup != nil {
		defer replayPaths.cleanup()
	}

	reporter := NewTuneReporter(options.quiet)

	if divergences, verifyErr := krakenmarket.CountCaptureBookDivergences(
		parent,
		options.replayFile,
	); verifyErr == nil && divergences > 0 {
		reporter.Phase(fmt.Sprintf(
			"WARNING: %s has %d book checksum divergences — fluid/depthflow go blind for those symbols; tuning scores are unreliable",
			options.replayFile,
			divergences,
		))
	}

	reporter.Phase(fmt.Sprintf(
		"Tuning replay %s — maximizing holdout wallet fitness (score − gate regret)",
		replayPaths.trainPath,
	))

	if len(replayPaths.holdoutPaths) > 0 {
		reporter.Phase(fmt.Sprintf(
			"Holdout: %s | overfit guard ≤ €%.2f train−holdout gap",
			strings.Join(replayPaths.holdoutPaths, ", "),
			options.maxTrainHoldoutGap,
		))
	}

	reporter.Phase("Step 1/3: profiling replay signals to learn category primitives…")
	profile, err := profileReplayPrimitives(parent, replayPaths.trainPath)

	if err != nil {
		return tuneRunResult{}, err
	}

	reporter.Phase(fmt.Sprintf(
		"Profile ready — %d categories seen on train replay",
		len(profile.Categories),
	))

	documentSearch, err := perspectives.NewDocumentSearch(profile, nil)

	if err != nil {
		return tuneRunResult{}, err
	}

	tunablesSearch := config.NewTunablesSearch(config.System, nil)
	rejectedOverfit := atomic.Int64{}
	rejectedNoProfit := atomic.Int64{}
	hasBest := false
	bestSelection := 0.0
	bestTrainScore := 0.0
	bestHoldoutScores := []float64(nil)
	bestGap := 0.0
	var bestConfig tuneCandidate
	var bestMu sync.Mutex

	reporter.Phase("Step 2/3: scoring current desk config as search starting point…")

	if err := seedTuneSearchBaseline(
		reporter,
		options,
		replayPaths.trainPath,
		replayPaths.holdoutPaths,
		options.stressHoldout,
		options.perturbTrain,
		options.evalWorkers,
		options.maxTrainHoldoutGap,
		documentSearch,
		tunablesSearch,
		&hasBest,
		&bestSelection,
		&bestTrainScore,
		&bestHoldoutScores,
		&bestGap,
		&bestConfig,
		&bestMu,
	); err != nil {
		reporter.Phase(fmt.Sprintf("Baseline skipped: %v", err))
	}

	if options.maxTrials > 0 {
		reporter.Phase(fmt.Sprintf(
			"Step 3/3: up to %d trials with %d workers — stops at limit or Ctrl+C",
			options.maxTrials,
			options.workers,
		))
	} else {
		reporter.Phase(fmt.Sprintf(
			"Step 3/3: continuous search with %d workers — press Ctrl+C to stop and save the current best",
			options.workers,
		))
	}

	trialsCompleted, interrupted := runTuneTrialSearch(
		parent,
		reporter,
		options,
		replayPaths,
		documentSearch,
		tunablesSearch,
		&rejectedOverfit,
		&rejectedNoProfit,
		tuneSearchState{
			hasBest:           &hasBest,
			bestSelection:     &bestSelection,
			bestTrainScore:    &bestTrainScore,
			bestHoldoutScores: &bestHoldoutScores,
			bestGap:           &bestGap,
			bestConfig:        &bestConfig,
			bestMu:            &bestMu,
		},
	)

	if interrupted {
		reporter.Phase("Ctrl+C received — finishing in-flight trials and saving the current best…")
	}

	if !hasBest {
		message := "no eligible candidates — selected splits need realized profitable trades; " +
			"widen --max-train-holdout-gap only if the rejects are overfit-gap rejects"
		reporter.Phase(message)

		return tuneRunResult{}, fmt.Errorf("%s", message)
	}

	overlay := bestConfig.tunables
	overlay.Apply(config.System)

	reporter.Phase("Writing best configuration to run artifacts…")

	_, err = persistTuneOutputs(
		reporter,
		options.output,
		options.perspectiveOutput,
		config.System,
		bestConfig.perspectives,
	)

	if err != nil {
		return tuneRunResult{}, err
	}

	payload, err := json.Marshal(map[string]any{
		"best_fitness_eur":         bestSelection,
		"best_train_fitness_eur":   bestTrainScore,
		"best_holdout_fitness_eur": bestHoldoutScores,
		"best_train_holdout_gap":   bestGap,
		"max_train_holdout_gap":    options.maxTrainHoldoutGap,
		"rejected_overfit":         rejectedOverfit.Load(),
		"rejected_no_profit":       rejectedNoProfit.Load(),
		"train_replay":             replayPaths.trainPath,
		"holdout_replays":          replayPaths.holdoutPaths,
		"stress_holdout":           options.stressHoldout,
		"output":                   options.output,
		"perspectives_output":      options.perspectiveOutput,
		"perspective_categories":   len(profile.Categories),
		"trials_completed":         trialsCompleted,
		"max_trials":               options.maxTrials,
		"stopped_by_user":          interrupted,
		"workers":                  options.workers,
		"eval_workers":             options.evalWorkers,
	})

	if err != nil {
		return tuneRunResult{}, err
	}

	stopReason := "stopped by user (Ctrl+C)"

	if !interrupted {
		if options.maxTrials > 0 {
			stopReason = fmt.Sprintf("completed %d trials", options.maxTrials)
		} else {
			stopReason = "search ended"
		}
	}

	reporter.Summary(fmt.Sprintf(
		"Done (%s) — best holdout wallet fitness €%.2f (train €%.2f, %d trials, %d overfit rejects, %d no-profit rejects)",
		stopReason,
		bestSelection,
		bestTrainScore,
		trialsCompleted,
		rejectedOverfit.Load(),
		rejectedNoProfit.Load(),
	))

	return tuneRunResult{payload: payload}, nil
}

func tuneOptionsFromCommand(cmd *cobra.Command) (tuneRunOptions, error) {
	replayFlag, err := cmd.Flags().GetString("replay")

	if err != nil {
		return tuneRunOptions{}, err
	}

	replayFile, err := requireReplayFile(replayFlag)

	if err != nil {
		return tuneRunOptions{}, err
	}

	if _, statErr := os.Stat(replayFile); statErr != nil {
		return tuneRunOptions{}, tuneReplayMissingMessage(replayFile)
	}

	holdoutFiles, err := cmd.Flags().GetStringArray("holdout")

	if err != nil {
		return tuneRunOptions{}, err
	}

	autoHoldout, err := cmd.Flags().GetBool("auto-holdout")

	if err != nil {
		return tuneRunOptions{}, err
	}

	walkForwardFolds, err := cmd.Flags().GetInt("walk-forward-folds")

	if err != nil {
		return tuneRunOptions{}, err
	}

	perturbTrain, err := cmd.Flags().GetBool("replay-perturb")

	if err != nil {
		return tuneRunOptions{}, err
	}

	stressHoldout, err := cmd.Flags().GetBool("stress-holdout")

	if err != nil {
		return tuneRunOptions{}, err
	}

	maxTrials, err := cmd.Flags().GetInt("iterations")

	if err != nil {
		return tuneRunOptions{}, err
	}

	if maxTrials < 0 {
		return tuneRunOptions{}, fmt.Errorf("--iterations must be >= 0 (0 = run until Ctrl+C)")
	}

	workers, err := cmd.Flags().GetInt("workers")

	if err != nil || workers <= 0 {
		workers = runtime.NumCPU()
	}

	output, err := cmd.Flags().GetString("output")

	if err != nil || strings.TrimSpace(output) == "" {
		output = config.DefaultTunedPath()
	}

	perspectiveOutput, err := cmd.Flags().GetString("perspectives-output")

	if err != nil || strings.TrimSpace(perspectiveOutput) == "" {
		perspectiveOutput = config.DefaultPerspectivePath()
	}

	maxGapFlag, err := cmd.Flags().GetFloat64("max-train-holdout-gap")

	if err != nil {
		return tuneRunOptions{}, err
	}

	quiet, err := cmd.Flags().GetBool("quiet")

	if err != nil {
		return tuneRunOptions{}, err
	}

	return tuneRunOptions{
		replayFile:         replayFile,
		holdoutFiles:       holdoutFiles,
		autoHoldout:        autoHoldout,
		walkForwardFolds:   walkForwardFolds,
		perturbTrain:       perturbTrain,
		stressHoldout:      stressHoldout,
		maxTrials:          maxTrials,
		workers:            workers,
		evalWorkers:        resolveTuneEvalWorkers(workers),
		output:             output,
		perspectiveOutput:  perspectiveOutput,
		maxTrainHoldoutGap: resolveMaxTrainHoldoutGap(maxGapFlag, config.System.WalletEUR),
		quiet:              quiet,
	}, nil
}

func seedTuneSearchBaseline(
	reporter *TuneReporter,
	options tuneRunOptions,
	trainReplay string,
	holdoutReplays []string,
	stressHoldout bool,
	perturbTrain bool,
	evalWorkers int,
	maxTrainHoldoutGap float64,
	documentSearch *perspectives.DocumentSearch,
	tunablesSearch *config.TunablesSearch,
	hasBest *bool,
	bestSelection *float64,
	bestTrainScore *float64,
	bestHoldoutScores *[]float64,
	bestGap *float64,
	bestConfig *tuneCandidate,
	bestMu *sync.Mutex,
) error {
	perspectivePath := config.PerspectiveLoadPath()
	document, err := loadBaselineDocument(perspectivePath)

	if err != nil {
		return err
	}

	candidate := tuneCandidate{
		tunables:     config.ExtractTunables(config.System),
		perspectives: &document,
	}
	scores, err := scoreTrial(
		trainReplay,
		holdoutReplays,
		stressHoldout,
		perturbTrain,
		1,
		evalWorkers,
		maxTrainHoldoutGap,
		candidate,
	)

	if err != nil {
		return err
	}

	reporter.BaselineScore(tuneTrialEvent{
		selection:    scores.selection,
		trainScore:   scores.trainScore,
		gap:          scores.gap,
		eligible:     scores.eligible,
		rejectReason: scores.rejectReason,
	})

	if !scores.eligible {
		return fmt.Errorf("baseline not eligible: %s", scores.rejectReason)
	}

	documentSearch.Observe(document, scores.selection)
	tunablesSearch.Observe(candidate.tunables, scores.selection)

	bestMu.Lock()
	*hasBest = true
	*bestSelection = scores.selection
	*bestTrainScore = scores.trainScore
	*bestHoldoutScores = append([]float64(nil), scores.holdoutScores...)
	*bestGap = scores.gap
	*bestConfig = snapshotTuneCandidate(document, candidate.tunables)
	bestMu.Unlock()

	if saveErr := saveTuneLeader(reporter, options, *bestConfig); saveErr != nil {
		return saveErr
	}

	reporter.Summary(fmt.Sprintf(
		"Baseline from %s — search will try to beat holdout €%.2f (train €%.2f)",
		perspectivePath,
		scores.selection,
		scores.trainScore,
	))

	return nil
}
