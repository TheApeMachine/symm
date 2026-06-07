package tune

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/viper"
	preasoning "github.com/theapemachine/symm/market/perspectives/reasoning"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/replay"
)

const (
	// defaultHoldoutFraction reserves the chronological tail of the capture for
	// out-of-sample scoring. The search never sees these rows; the chosen forest
	// must reproduce on them before it may be written to the live playbook.
	defaultHoldoutFraction = 0.25
	// defaultMaxHoldoutDecay is the largest tolerated drop in per-trade PnL from
	// train to holdout (0.5 = holdout keeps at least half the train edge).
	defaultMaxHoldoutDecay = 0.5
	// defaultEmbargo is the gap skipped after the split so latched reasoning
	// chains and the prediction-feedback horizon cannot straddle train and test.
	defaultEmbargo = time.Minute
	// minHoldoutRows is the smallest train or test slice worth validating on;
	// below it the holdout is disabled (and the candidate is NOT written, since
	// it cannot be validated).
	minHoldoutRows = 200
	// stressFeeMultiple rescores the survivor with fees and slippage inflated by
	// this factor; an edge that dies under +50% costs is noise, not edge.
	stressFeeMultiple = 1.5
)

func holdoutFraction() float64 {
	if viper.IsSet("optimizer.tune.holdout_fraction") {
		fraction := viper.GetFloat64("optimizer.tune.holdout_fraction")

		if fraction >= 0 && fraction < 1 {
			return fraction
		}
	}

	return defaultHoldoutFraction
}

func maxHoldoutDecay() float64 {
	if value := viper.GetFloat64("optimizer.tune.max_holdout_decay"); value > 0 {
		return value
	}

	return defaultMaxHoldoutDecay
}

func holdoutEmbargo() time.Duration {
	if horizon := viper.GetDuration("story.prediction.horizon"); horizon > 0 {
		return horizon
	}

	return defaultEmbargo
}

/*
normalizeRowOrder enforces the chronological contract every temporal predicate
and snapshot window depends on. Capture rows are written by concurrent signal
goroutines and arrive with small inversions (~0.25% in measured tapes); replay
must not interpret tape order as time order when they disagree.
*/
func normalizeRowOrder(rows []ptypes.Measurement) []ptypes.Measurement {
	inversions := 0

	for index := 1; index < len(rows); index++ {
		if rows[index].At.Before(rows[index-1].At) {
			inversions++
		}
	}

	if inversions == 0 {
		return rows
	}

	log.TuneLog("capture has %d out-of-order rows — stable-sorting by timestamp", inversions)
	sort.SliceStable(rows, func(left, right int) bool {
		return rows[left].At.Before(rows[right].At)
	})

	return rows
}

/*
tapeSanity refuses tapes replay cannot trade on and reports per-source coverage so
a playbook gated on a source with no rows is visible before hours of search. The
2026-06-07 capture (17.6M rows, zero book depth after the quote cache was severed
from the bus) would previously have produced a silent zero-trade tune.
*/
func tapeSanity(rows []ptypes.Measurement) error {
	if len(rows) == 0 {
		return fmt.Errorf("optimizer/tune: empty measurement tape")
	}

	bookRows := 0
	spreadRows := 0
	perSource := make(map[string]int)

	for index := range rows {
		if rows[index].HasBookDepth() {
			bookRows++
		}

		if rows[index].SpreadBPS > 0 || (rows[index].Bid > 0 && rows[index].Ask > 0) {
			spreadRows++
		}

		perSource[rows[index].Source.String()]++
	}

	for source, count := range perSource {
		log.TuneLog("tape coverage: source=%s rows=%d (%.2f%%)", source, count, 100*float64(count)/float64(len(rows)))
	}

	if bookRows == 0 {
		return fmt.Errorf(
			"optimizer/tune: no rows carry book depth — replay cannot fill a single entry on this tape; re-record with the quote cache attached (see runs/capture.jsonl + SYSTEM_REVIEW.md §1)",
		)
	}

	log.TuneLog(
		"tape sanity: %d/%d rows with book depth (%.2f%%), %d with spread info",
		bookRows, len(rows), 100*float64(bookRows)/float64(len(rows)), spreadRows,
	)

	return nil
}

/*
splitHoldout reserves the chronological tail as the out-of-sample slice and skips
an embargo window after the boundary so cross-boundary state cannot leak.
*/
func splitHoldout(
	rows []ptypes.Measurement,
	fraction float64,
	embargo time.Duration,
) (train []ptypes.Measurement, test []ptypes.Measurement) {
	if fraction <= 0 || len(rows) == 0 {
		return rows, nil
	}

	cut := int(float64(len(rows)) * (1 - fraction))

	if cut <= 0 || cut >= len(rows) {
		return rows, nil
	}

	train = rows[:cut]
	boundary := rows[cut-1].At
	testStart := cut

	if embargo > 0 && !boundary.IsZero() {
		for testStart < len(rows) {
			at := rows[testStart].At

			if at.IsZero() || at.Sub(boundary) >= embargo {
				break
			}

			testStart++
		}
	}

	if testStart >= len(rows) {
		return train, nil
	}

	return train, rows[testStart:]
}

/*
scoreForestOnRows replays one forest over a row slice with the given costs.
*/
func scoreForestOnRows(
	ctx context.Context,
	forest []preasoning.Thought,
	rows []ptypes.Measurement,
	costs replay.ReplayCosts,
	workers int,
) (replay.ReplayResult, error) {
	if len(forest) == 0 || len(rows) == 0 {
		return replay.ReplayResult{}, nil
	}

	attributedCosts := costs
	attributedCosts.CollectAttribution = true

	tape, err := replay.PrecompileTapeWorkers(rows, workers)

	if err != nil {
		return replay.ReplayResult{}, err
	}

	return replay.NewThoughtSimulation(ctx, forest, tape, attributedCosts).Result(), nil
}

/*
logPerSetup prints the chosen forest's per-setup attribution on the train slice,
so a tune's output reads as a portfolio of named setups — each with its own
trades, win rate and realized PnL — instead of one anonymous total.
*/
func logPerSetup(
	ctx context.Context,
	forest []preasoning.Thought,
	rows []ptypes.Measurement,
	costs replay.ReplayCosts,
	workers int,
) {
	if len(forest) == 0 || len(rows) == 0 {
		return
	}

	result, err := scoreForestOnRows(ctx, forest, rows, costs, workers)

	if err != nil || len(result.PerStrategy) == 0 {
		return
	}

	for name, setup := range result.PerStrategy {
		winPct := 0.0

		if setup.Trades > 0 {
			winPct = 100 * float64(setup.Wins) / float64(setup.Trades)
		}

		log.TuneLog(
			"setup %s: trades=%d win=%.0f%% realized_eur=%+.4f avg_hold=%.0fs (train)",
			name,
			setup.Trades,
			winPct,
			setup.RealizedEUR,
			setup.AvgHoldSeconds,
		)
	}
}

func stressedCosts(costs replay.ReplayCosts) replay.ReplayCosts {
	stressed := costs
	stressed.TakerFeePct *= stressFeeMultiple
	stressed.MakerFeePct *= stressFeeMultiple
	stressed.SlippagePct *= stressFeeMultiple

	return stressed
}

/*
holdoutVerdict is the out-of-sample report card for the search's chosen forest.
*/
type holdoutVerdict struct {
	Enabled        bool
	TestTrades     int
	TestRealized   float64
	TestReturn     float64
	Decay          float64
	StressRealized float64
	Publish        bool
	Reason         string
}

func perTrade(realized float64, trades int) float64 {
	if trades <= 0 {
		return 0
	}

	return realized / float64(trades)
}

/*
evaluateHoldout decides whether the train-selected forest earned the right to be
written to the live playbook: it must close trades out-of-sample, keep enough of
its per-trade edge (holdoutDecay gate), and survive a +50% cost stress. With the
holdout disabled (fraction 0) it defers to the legacy in-sample gate, loudly.
*/
func evaluateHoldout(
	ctx context.Context,
	forest []preasoning.Thought,
	trainRealized float64,
	trainTrades int,
	testRows []ptypes.Measurement,
	costs replay.ReplayCosts,
	workers int,
) holdoutVerdict {
	if len(testRows) < minHoldoutRows {
		return holdoutVerdict{
			Enabled: false,
			Publish: true,
			Reason:  fmt.Sprintf("holdout disabled (%d test rows < %d) — IN-SAMPLE ONLY, treat the playbook as unvalidated", len(testRows), minHoldoutRows),
		}
	}

	verdict := holdoutVerdict{Enabled: true}

	testResult, err := scoreForestOnRows(ctx, forest, testRows, costs, workers)

	if err != nil {
		verdict.Reason = fmt.Sprintf("holdout replay failed: %v", err)

		return verdict
	}

	verdict.TestTrades = testResult.ClosedTrades
	verdict.TestRealized = testResult.RealizedEUR
	verdict.TestReturn = testResult.Score
	verdict.Decay = replay.HoldoutDecay(
		perTrade(trainRealized, trainTrades),
		perTrade(testResult.RealizedEUR, testResult.ClosedTrades),
	)

	if testResult.ClosedTrades <= 0 {
		verdict.Reason = "no closed round trips on the holdout"

		return verdict
	}

	if testResult.RealizedEUR <= 0 {
		verdict.Reason = fmt.Sprintf("holdout realized %.4f ≤ 0", testResult.RealizedEUR)

		return verdict
	}

	if limit := maxHoldoutDecay(); verdict.Decay > limit {
		verdict.Reason = fmt.Sprintf("holdout per-trade decay %.2f exceeds %.2f", verdict.Decay, limit)

		return verdict
	}

	stressResult, err := scoreForestOnRows(ctx, forest, testRows, stressedCosts(costs), workers)

	if err != nil {
		verdict.Reason = fmt.Sprintf("stress replay failed: %v", err)

		return verdict
	}

	verdict.StressRealized = stressResult.RealizedEUR

	if stressResult.RealizedEUR <= 0 {
		verdict.Reason = fmt.Sprintf(
			"edge dies under %.1fx costs (stressed realized %.4f)",
			stressFeeMultiple, stressResult.RealizedEUR,
		)

		return verdict
	}

	verdict.Publish = true
	verdict.Reason = "holdout passed"

	return verdict
}
