package cmd

import (
	"fmt"
	"io"
	"os"
	"sync"
)

/*
TuneReporter prints human-readable tune progress to stderr while JSON stays on stdout.
*/
type TuneReporter struct {
	quiet          bool
	writer         io.Writer
	mu             sync.Mutex
	total          int
	finished       int
	bestHoldout    float64
	bestTrain      float64
	hasCurrentBest bool
}

func NewTuneReporter(quiet bool) *TuneReporter {
	writer := io.Writer(os.Stderr)

	if quiet {
		writer = io.Discard
	}

	return &TuneReporter{
		quiet:  quiet,
		writer: writer,
	}
}

func (reporter *TuneReporter) Phase(message string) {
	reporter.println(message)
}

func (reporter *TuneReporter) SetCurrentBest(holdoutFitness float64, trainFitness float64) {
	reporter.mu.Lock()
	reporter.bestHoldout = holdoutFitness
	reporter.bestTrain = trainFitness
	reporter.hasCurrentBest = true
	reporter.mu.Unlock()

	reporter.println(formatCurrentBestLine(holdoutFitness, trainFitness))
}

func (reporter *TuneReporter) TrialResult(event tuneTrialEvent) {
	reporter.mu.Lock()
	reporter.finished++
	finished := reporter.finished
	total := reporter.total

	if event.newBest {
		reporter.bestHoldout = event.currentBestHoldout
		reporter.bestTrain = event.currentBestTrain
		reporter.hasCurrentBest = true
	}

	bestHoldout := reporter.bestHoldout
	bestTrain := reporter.bestTrain
	hasCurrentBest := reporter.hasCurrentBest
	reporter.mu.Unlock()

	status := "eligible (ranked by holdout)"

	if !event.eligible {
		status = "rejected (train >> holdout, overfit)"
	}

	line := formatTrialLine(finished, total, event.selection, event.trainScore, event.gap, status)
	reporter.println(line)

	if event.newBest && event.eligible {
		reporter.println("  ★ updated leader (this config will be saved if still #1 at the end)")
	}

	if hasCurrentBest {
		reporter.println(formatCurrentBestLine(bestHoldout, bestTrain))
	}
}

func formatCurrentBestLine(holdoutFitness float64, trainFitness float64) string {
	return fmt.Sprintf(
		"  ► CURRENT BEST → holdout €%.2f | train €%.2f (highest holdout wins; this is what gets written to runs/)",
		holdoutFitness,
		trainFitness,
	)
}

func (reporter *TuneReporter) BaselineScore(event tuneTrialEvent) {
	status := "eligible baseline"

	if !event.eligible {
		status = "ineligible baseline (skipped as incumbent)"
	}

	reporter.println(fmt.Sprintf(
		"[baseline] holdout €%.2f | train €%.2f | gap €%.2f | %s",
		event.selection,
		event.trainScore,
		event.gap,
		status,
	))

	if event.eligible {
		reporter.SetCurrentBest(event.selection, event.trainScore)
	}
}

func (reporter *TuneReporter) SetTotal(total int) {
	reporter.mu.Lock()
	reporter.total = total
	reporter.mu.Unlock()
}

func (reporter *TuneReporter) Summary(message string) {
	reporter.println(message)
}

func (reporter *TuneReporter) println(message string) {
	reporter.mu.Lock()
	defer reporter.mu.Unlock()

	_, _ = fmt.Fprintln(reporter.writer, message)
}

func formatTrialLine(
	finished int,
	total int,
	holdout float64,
	train float64,
	gap float64,
	status string,
) string {
	if total > 0 {
		return fmt.Sprintf(
			"[trial %d/%d] holdout €%.2f | train €%.2f | gap €%.2f | %s",
			finished,
			total,
			holdout,
			train,
			gap,
			status,
		)
	}

	return fmt.Sprintf(
		"[trial %d] holdout €%.2f | train €%.2f | gap €%.2f | %s",
		finished,
		holdout,
		train,
		gap,
		status,
	)
}

type tuneTrialEvent struct {
	selection          float64
	trainScore         float64
	gap                float64
	eligible           bool
	newBest            bool
	currentBestHoldout float64
	currentBestTrain   float64
}
