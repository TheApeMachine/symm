package tune

import (
	"context"
	"fmt"

	preasoning "github.com/theapemachine/symm/market/perspectives/reasoning"
	ptypes "github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/market/quote"
	"github.com/theapemachine/symm/optimizer/io"
	"github.com/theapemachine/symm/optimizer/log"
	"github.com/theapemachine/symm/optimizer/reasoning"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

/*
TuneMeasurements grows reasoning forests from a recorded measurement tape and writes
the best one it finds to the playbook. Realized round-trip PnL on the replay is the
only objective; the tree's depth and breadth are discovered, not preset.
*/
func TuneMeasurements(
	ctx context.Context,
	rows []ptypes.Measurement,
	options types.TuneOptions,
) (types.SessionSummary, error) {
	if options.MaxMeasurements < 0 {
		return types.SessionSummary{}, fmt.Errorf(
			"optimizer/tune: max measurements must be non-negative, got %d",
			options.MaxMeasurements,
		)
	}

	if options.MaxMeasurements > 0 && len(rows) > options.MaxMeasurements {
		rows = rows[:options.MaxMeasurements]
	}

	costs := replay.DefaultReplayCosts()

	if err := attachInstrumentRules(ctx, &costs, options.InstrumentRules); err != nil {
		return types.SessionSummary{}, err
	}

	measurementCount := len(rows)
	rows = fundableRows(rows, costs.WalletCurrency)
	minRoundTrips := effectiveMinRoundTrips(options.MinRoundTrips, rows, costs.WalletCurrency)

	if len(rows) != measurementCount {
		log.TuneLog(
			"filtered measurements to %d fundable %s rows from %d total",
			len(rows),
			costs.WalletCurrency,
			measurementCount,
		)
	}

	rows = normalizeRowOrder(rows)

	if err := tapeSanity(rows); err != nil {
		return types.SessionSummary{}, err
	}

	fundableCount := len(rows)
	trainRows, testRows := splitHoldout(rows, holdoutFraction(), holdoutEmbargo())

	if len(testRows) > 0 && len(testRows) < minHoldoutRows {
		// A holdout too small to validate on must not also rob the search of its
		// tail — train on everything and mark the result unvalidated instead.
		log.TuneLog(
			"holdout tail too small to validate (%d rows < %d) — training on the full tape",
			len(testRows), minHoldoutRows,
		)
		trainRows, testRows = rows, nil
	}

	if len(testRows) > 0 {
		log.TuneLog(
			"walk-forward split: %d train rows, %d holdout rows (embargo %s)",
			len(trainRows), len(testRows), holdoutEmbargo(),
		)
	} else {
		log.TuneLog("WARNING: holdout disabled — selection is in-sample only")
	}

	rows = trainRows

	log.TuneLog("searching reasoning forests over %d rows", len(rows))

	config := reasoning.SearchConfig{
		BeamWidth:     options.BeamWidth,
		MaxRounds:     options.MaxRounds,
		MaxNodes:      options.MaxNodes,
		MinRoundTrips: minRoundTrips,
		Workers:       options.Workers,
		OnProgress: func(progress reasoning.SearchProgress) {
			log.TuneLog("%s", progress.Message())
		},
		OnNewBest: func(candidate reasoning.Candidate) {
			if options.OnCandidate == nil || candidate.Trades <= 0 {
				return
			}

			options.OnCandidate(types.CandidateScore{
				Candidate:       0,
				Score:           candidate.Score,
				ReturnFraction:  candidate.Return,
				RealizedEUR:     candidate.RealizedEUR,
				StartingCapital: candidate.StartingCapital,
				ClosedTrades:    candidate.Trades,
				Depth:           forestDepth(candidate.Forest),
				Strategies:      strategyCount(candidate.Forest),
				Thoughts:        candidate.Forest,
			})
		},
	}

	result, err := reasoning.Search(ctx, rows, costs, config)

	if err != nil {
		return types.SessionSummary{}, err
	}

	best := result.Best

	strategies := strategyCount(best.Forest)
	depth := forestDepth(best.Forest)

	if options.OnCandidate != nil {
		options.OnCandidate(types.CandidateScore{
			Candidate:       result.Evaluated,
			Score:           best.Score,
			ReturnFraction:  best.Return,
			RealizedEUR:     best.RealizedEUR,
			StartingCapital: best.StartingCapital,
			ClosedTrades:    best.Trades,
			Depth:           depth,
			Strategies:      strategies,
			Thoughts:        best.Forest,
		})
	}

	if options.OnBest != nil {
		options.OnBest(types.BestTree{
			Iteration: result.Evaluated,
			Score:     best.Score,
			Return:    best.Return,
			Trades:    best.Trades,
			Nodes:     best.Nodes,
			Thoughts:  best.Forest,
		})
	}

	logPerSetup(ctx, best.Forest, rows, costs, options.Workers)

	verdict := evaluateHoldout(
		ctx,
		best.Forest,
		best.RealizedEUR,
		best.Trades,
		testRows,
		costs,
		options.Workers,
	)

	if verdict.Enabled {
		log.TuneLog(
			"holdout: trades=%d realized_eur=%.4f return=%.6f decay=%.3f stressed_eur=%.4f — %s",
			verdict.TestTrades,
			verdict.TestRealized,
			verdict.TestReturn,
			verdict.Decay,
			verdict.StressRealized,
			verdict.Reason,
		)
	} else {
		log.TuneLog("%s", verdict.Reason)
	}

	if options.OutputPath != "" && shouldWrite(best) && verdict.Publish {
		if err := io.WriteThoughts(options.OutputPath, best.Forest); err != nil {
			return types.SessionSummary{}, err
		}
	} else if shouldWrite(best) && !verdict.Publish {
		log.TuneLog("not writing candidate: %s", verdict.Reason)
	}

	return types.SessionSummary{
		MeasurementCount:     measurementCount,
		FundableMeasurements: fundableCount,
		MinRoundTrips:        minRoundTrips,
		Strategies:           strategies,
		Nodes:                best.Nodes,
		Trades:               best.Trades,
		Evaluated:            result.Evaluated,
		BestReturn:           best.Return,
		BestScore:            best.Score,
		HoldoutTrades:        verdict.TestTrades,
		HoldoutReturn:        verdict.TestReturn,
		HoldoutDecay:         verdict.Decay,
		HoldoutPublished:     verdict.Publish,
	}, nil
}

func shouldWrite(best reasoning.Candidate) bool {
	if len(best.Forest) == 0 {
		log.TuneLog(
			"not writing candidate: empty forest strategies=%d nodes=%d trades=%d score=%.6f return=%.6f",
			len(best.Forest),
			best.Nodes,
			best.Trades,
			best.Score,
			best.Return,
		)

		return false
	}

	if best.Score <= 0 || best.Return <= 0 {
		log.TuneLog(
			"not writing candidate: non-positive score/return score=%.6f return=%.6f trades=%d",
			best.Score,
			best.Return,
			best.Trades,
		)

		return false
	}

	return true
}

func fundableRows(
	rows []ptypes.Measurement,
	walletCurrency string,
) []ptypes.Measurement {
	currency := quote.NormalizeCurrency(walletCurrency)

	if currency == "" {
		return rows
	}

	filtered := make([]ptypes.Measurement, 0, len(rows))

	for _, row := range rows {
		if row.Symbol == "" {
			filtered = append(filtered, row)

			continue
		}

		if quote.SymbolMatchesCurrency(row.Symbol, currency) {
			filtered = append(filtered, row)
		}
	}

	return filtered
}

func fundableSymbolCount(
	rows []ptypes.Measurement,
	walletCurrency string,
) int {
	currency := quote.NormalizeCurrency(walletCurrency)

	if currency == "" {
		return 0
	}

	symbols := make(map[string]bool)

	for _, row := range rows {
		if row.Symbol == "" {
			continue
		}

		if quote.SymbolMatchesCurrency(row.Symbol, currency) {
			symbols[row.Symbol] = true
		}
	}

	return len(symbols)
}

// forestDepth is the deepest Then-chain in the forest.
func forestDepth(forest []preasoning.Thought) int {
	deepest := 0

	var walk func(thought preasoning.Thought, depth int)
	walk = func(thought preasoning.Thought, depth int) {
		if depth > deepest {
			deepest = depth
		}

		for _, child := range thought.Then {
			walk(child, depth+1)
		}
	}

	for _, thought := range forest {
		walk(thought, 1)
	}

	return deepest
}

// strategyCount is the number of root branches that reach an entry.
func strategyCount(forest []preasoning.Thought) int {
	var reachesEntry func(thought preasoning.Thought) bool
	reachesEntry = func(thought preasoning.Thought) bool {
		if preasoning.IsEntryAction(thought.Do.Type) {
			return true
		}

		for _, child := range thought.Then {
			if reachesEntry(child) {
				return true
			}
		}

		return false
	}

	count := 0

	for _, thought := range forest {
		if reachesEntry(thought) {
			count++
		}
	}

	return count
}
