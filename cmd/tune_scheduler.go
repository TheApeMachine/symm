package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

type tuneSearchState struct {
	hasBest           *bool
	bestSelection     *float64
	bestTrainScore    *float64
	bestHoldoutScores *[]float64
	bestGap           *float64
	bestConfig        *tuneCandidate
	bestMu            *sync.Mutex
}

type tuneScoredTrial struct {
	document perspectives.Document
	tunables config.Tunables
	scores   trialScores
	err      error
}

func runTuneTrialSearch(
	parent context.Context,
	reporter *TuneReporter,
	options tuneRunOptions,
	replayPaths tuneReplayPaths,
	documentSearch *perspectives.DocumentSearch,
	tunablesSearch *config.TunablesSearch,
	rejectedOverfit *atomic.Int64,
	rejectedNoProfit *atomic.Int64,
	state tuneSearchState,
) (int64, bool) {
	reporter.SetTotal(options.maxTrials)

	poolConfig := qpool.NewConfig()
	poolConfig.Scaler = nil
	trialPool := qpool.NewQ(context.Background(), options.workers, options.workers, poolConfig)
	defer trialPool.Close()

	results := make(chan tuneScoredTrial, options.workers)
	done := parent.Done()
	inFlight := 0
	nextTrial := 0
	trialsCompleted := int64(0)
	submitting := true

	for submitting || inFlight > 0 {
		for submitting && inFlight < options.workers && shouldSubmitTuneTrial(options, nextTrial) {
			submitTuneTrial(
				trialPool,
				results,
				options,
				replayPaths,
				documentSearch,
				tunablesSearch,
				nextTrial,
			)
			nextTrial++
			inFlight++
		}

		if !shouldSubmitTuneTrial(options, nextTrial) {
			submitting = false
		}

		if inFlight == 0 {
			break
		}

		select {
		case result := <-results:
			inFlight--

			if handleTuneTrialResult(
				reporter,
				options,
				documentSearch,
				tunablesSearch,
				rejectedOverfit,
				rejectedNoProfit,
				state,
				result,
			) {
				trialsCompleted++
			}
		case <-done:
			submitting = false
			done = nil
		}
	}

	return trialsCompleted, errors.Is(parent.Err(), context.Canceled)
}

func shouldSubmitTuneTrial(options tuneRunOptions, nextTrial int) bool {
	return options.maxTrials <= 0 || nextTrial < options.maxTrials
}

func submitTuneTrial(
	trialPool *qpool.Q,
	results chan<- tuneScoredTrial,
	options tuneRunOptions,
	replayPaths tuneReplayPaths,
	documentSearch *perspectives.DocumentSearch,
	tunablesSearch *config.TunablesSearch,
	trialIndex int,
) {
	document := documentSearch.Next()
	tunables := tunablesSearch.Next()
	candidate := tuneCandidate{
		tunables:     tunables,
		perspectives: &document,
	}
	resultChannel := trialPool.ScheduleFast(context.Background(), func(context.Context) (any, error) {
		return scoreTrial(
			replayPaths.trainPath,
			replayPaths.holdoutPaths,
			options.stressHoldout,
			options.perturbTrain,
			tuneTrialPerturbSeed(trialIndex),
			options.evalWorkers,
			options.maxTrainHoldoutGap,
			candidate,
		)
	})

	go func() {
		qvalue := <-resultChannel
		result := tuneScoredTrial{
			document: document,
			tunables: tunables,
		}

		if qvalue != nil {
			result.err = qvalue.Error

			if scores, ok := qvalue.Value.(trialScores); ok {
				result.scores = scores
			}
		}

		results <- result
	}()
}

func tuneTrialPerturbSeed(trialIndex int) int64 {
	return int64(trialIndex + 2)
}

func handleTuneTrialResult(
	reporter *TuneReporter,
	options tuneRunOptions,
	documentSearch *perspectives.DocumentSearch,
	tunablesSearch *config.TunablesSearch,
	rejectedOverfit *atomic.Int64,
	rejectedNoProfit *atomic.Int64,
	state tuneSearchState,
	result tuneScoredTrial,
) bool {
	if result.err != nil {
		reporter.println(fmt.Sprintf("  trial error: %v", result.err))

		return false
	}

	candidate := tuneCandidate{
		tunables:     result.tunables,
		perspectives: &result.document,
	}
	newBest := false

	if result.scores.eligible {
		newBest = observeEligibleTuneTrial(
			reporter,
			options,
			documentSearch,
			tunablesSearch,
			state,
			candidate,
			result.scores,
		)
	} else {
		countRejectedTuneTrial(result.scores.rejectReason, rejectedOverfit, rejectedNoProfit)
	}

	state.bestMu.Lock()
	currentBestHoldout := *state.bestSelection
	currentBestTrain := *state.bestTrainScore
	state.bestMu.Unlock()

	reporter.TrialResult(tuneTrialEvent{
		selection:          result.scores.selection,
		trainScore:         result.scores.trainScore,
		gap:                result.scores.gap,
		eligible:           result.scores.eligible,
		rejectReason:       result.scores.rejectReason,
		newBest:            newBest,
		currentBestHoldout: currentBestHoldout,
		currentBestTrain:   currentBestTrain,
	})

	return true
}

func countRejectedTuneTrial(
	reason string,
	rejectedOverfit *atomic.Int64,
	rejectedNoProfit *atomic.Int64,
) {
	switch reason {
	case tuneRejectOverfit:
		rejectedOverfit.Add(1)
	case tuneRejectNoProfit:
		rejectedNoProfit.Add(1)
	}
}

func observeEligibleTuneTrial(
	reporter *TuneReporter,
	options tuneRunOptions,
	documentSearch *perspectives.DocumentSearch,
	tunablesSearch *config.TunablesSearch,
	state tuneSearchState,
	candidate tuneCandidate,
	scores trialScores,
) bool {
	documentSearch.Observe(*candidate.perspectives, scores.selection)
	tunablesSearch.Observe(candidate.tunables, scores.selection)
	state.bestMu.Lock()

	if *state.hasBest && !betterTuneCandidate(
		scores.selection,
		scores.trainScore,
		*state.bestSelection,
		*state.bestTrainScore,
	) {
		state.bestMu.Unlock()

		return false
	}

	*state.hasBest = true
	*state.bestSelection = scores.selection
	*state.bestTrainScore = scores.trainScore
	*state.bestHoldoutScores = append([]float64(nil), scores.holdoutScores...)
	*state.bestGap = scores.gap
	*state.bestConfig = snapshotTuneCandidate(*candidate.perspectives, candidate.tunables)
	bestConfig := *state.bestConfig
	state.bestMu.Unlock()

	if saveErr := saveTuneLeader(reporter, options, bestConfig); saveErr != nil {
		reporter.println(fmt.Sprintf("  save leader failed: %v", saveErr))
	}

	return true
}
