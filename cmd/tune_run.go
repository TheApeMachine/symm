package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

type tuneRunOptions struct {
	replayFile         string
	holdoutFiles       []string
	autoHoldout        bool
	stressHoldout      bool
	maxTrials          int
	workers            int
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

	replayPaths, err := resolveTuneReplayPaths(options.replayFile, options.holdoutFiles, options.autoHoldout)

	if err != nil {
		return tuneRunResult{}, err
	}

	if replayPaths.cleanup != nil {
		defer replayPaths.cleanup()
	}

	reporter := NewTuneReporter(options.quiet)
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
		replayPaths.trainPath,
		replayPaths.holdoutPaths,
		options.stressHoldout,
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

	reporter.SetTotal(options.maxTrials)

	jobs := make(chan int, options.workers*2)
	var waitGroup sync.WaitGroup
	var trialsCompleted atomic.Int64

	go dispatchTuneTrials(parent, options.maxTrials, jobs)

	for worker := 0; worker < options.workers; worker++ {
		waitGroup.Go(func() {
			for range jobs {
				if parent.Err() != nil {
					return
				}
				document := documentSearch.Next()
				candidate := tuneCandidate{
					tunables:     tunablesSearch.Next(),
					perspectives: &document,
				}
				scores, scoreErr := scoreTrial(
					replayPaths.trainPath,
					replayPaths.holdoutPaths,
					options.stressHoldout,
					options.maxTrainHoldoutGap,
					candidate,
				)

				if scoreErr != nil {
					reporter.println(fmt.Sprintf("  trial error: %v", scoreErr))
					continue
				}

				newBest := false

				if scores.eligible {
					documentSearch.Observe(document, scores.selection)
					tunablesSearch.Observe(candidate.tunables, scores.selection)

					bestMu.Lock()

					if betterTuneCandidate(
						scores.selection,
						scores.trainScore,
						bestSelection,
						bestTrainScore,
					) {
						hasBest = true
						bestSelection = scores.selection
						bestTrainScore = scores.trainScore
						bestHoldoutScores = append([]float64(nil), scores.holdoutScores...)
						bestGap = scores.gap
						bestConfig = candidate
						newBest = true
					}

					bestMu.Unlock()
				} else {
					rejectedOverfit.Add(1)
				}

				bestMu.Lock()
				currentBestHoldout := bestSelection
				currentBestTrain := bestTrainScore
				bestMu.Unlock()

				reporter.TrialResult(tuneTrialEvent{
					selection:          scores.selection,
					trainScore:         scores.trainScore,
					gap:                scores.gap,
					eligible:           scores.eligible,
					newBest:            newBest,
					currentBestHoldout: currentBestHoldout,
					currentBestTrain:   currentBestTrain,
				})
				trialsCompleted.Add(1)
			}
		})
	}

	waitGroup.Wait()

	interrupted := errors.Is(parent.Err(), context.Canceled)

	if interrupted {
		reporter.Phase("Ctrl+C received — finishing in-flight trials and saving the current best…")
	}

	if !hasBest {
		return tuneRunResult{}, fmt.Errorf("no eligible candidates — widen --max-train-holdout-gap or add holdout data")
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
		"train_replay":             replayPaths.trainPath,
		"holdout_replays":          replayPaths.holdoutPaths,
		"stress_holdout":           options.stressHoldout,
		"output":                   options.output,
		"perspectives_output":      options.perspectiveOutput,
		"perspective_categories":   len(profile.Categories),
		"trials_completed":         trialsCompleted.Load(),
		"max_trials":               options.maxTrials,
		"stopped_by_user":          interrupted,
		"workers":                  options.workers,
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
		"Done (%s) — best holdout wallet fitness €%.2f (train €%.2f, %d trials, %d overfit rejects)",
		stopReason,
		bestSelection,
		bestTrainScore,
		trialsCompleted.Load(),
		rejectedOverfit.Load(),
	))

	return tuneRunResult{payload: payload}, nil
}

func dispatchTuneTrials(parent context.Context, maxTrials int, jobs chan<- int) {
	defer close(jobs)

	trialIndex := 0

	for {
		if parent.Err() != nil {
			return
		}

		if maxTrials > 0 && trialIndex >= maxTrials {
			return
		}

		select {
		case <-parent.Done():
			return
		case jobs <- trialIndex:
			trialIndex++
		}
	}
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
		stressHoldout:      stressHoldout,
		maxTrials:          maxTrials,
		workers:            workers,
		output:             output,
		perspectiveOutput:  perspectiveOutput,
		maxTrainHoldoutGap: resolveMaxTrainHoldoutGap(maxGapFlag, config.System.WalletEUR),
		quiet:              quiet,
	}, nil
}

func seedTuneSearchBaseline(
	reporter *TuneReporter,
	trainReplay string,
	holdoutReplays []string,
	stressHoldout bool,
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
	scores, err := scoreTrial(trainReplay, holdoutReplays, stressHoldout, maxTrainHoldoutGap, candidate)

	if err != nil {
		return err
	}

	reporter.BaselineScore(tuneTrialEvent{
		selection:  scores.selection,
		trainScore: scores.trainScore,
		gap:        scores.gap,
		eligible:   scores.eligible,
	})

	if !scores.eligible {
		return fmt.Errorf("baseline not eligible (train−holdout gap €%.2f)", scores.gap)
	}

	documentSearch.Observe(document, scores.selection)
	tunablesSearch.Observe(candidate.tunables, scores.selection)

	bestMu.Lock()
	*hasBest = true
	*bestSelection = scores.selection
	*bestTrainScore = scores.trainScore
	*bestHoldoutScores = append([]float64(nil), scores.holdoutScores...)
	*bestGap = scores.gap
	*bestConfig = candidate
	bestMu.Unlock()

	reporter.Summary(fmt.Sprintf(
		"Baseline from %s — search will try to beat holdout €%.2f (train €%.2f)",
		perspectivePath,
		scores.selection,
		scores.trainScore,
	))

	return nil
}
