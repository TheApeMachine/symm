package tune

import (
	"context"

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
	costs := replay.DefaultReplayCosts()
	measurementCount := len(rows)
	rows = fundableRows(rows, costs.WalletCurrency)
	minRoundTrips := options.MinRoundTrips

	if minRoundTrips <= 0 {
		minRoundTrips = fundableSymbolCount(rows, costs.WalletCurrency)
	}

	if len(rows) != measurementCount {
		log.TuneLog(
			"filtered measurements to %d fundable %s rows from %d total",
			len(rows),
			costs.WalletCurrency,
			measurementCount,
		)
	}

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
				Candidate:    0,
				Score:        candidate.Return,
				ClosedTrades: candidate.Trades,
				Depth:        forestDepth(candidate.Forest),
				Strategies:   strategyCount(candidate.Forest),
				Thoughts:     candidate.Forest,
			})
		},
	}

	result := reasoning.Search(ctx, rows, costs, config)
	best := result.Best

	strategies := strategyCount(best.Forest)
	depth := forestDepth(best.Forest)

	if options.OnCandidate != nil {
		options.OnCandidate(types.CandidateScore{
			Candidate:    result.Evaluated,
			Score:        best.Return,
			ClosedTrades: best.Trades,
			Depth:        depth,
			Strategies:   strategies,
			Thoughts:     best.Forest,
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

	if options.OutputPath != "" && shouldWrite(best, minRoundTrips) {
		if err := io.WriteThoughts(options.OutputPath, best.Forest); err != nil {
			return types.SessionSummary{}, err
		}
	}

	return types.SessionSummary{
		MeasurementCount:     measurementCount,
		FundableMeasurements: len(rows),
		MinRoundTrips:        minRoundTrips,
		Strategies:           strategies,
		Nodes:                best.Nodes,
		Trades:               best.Trades,
		Evaluated:            result.Evaluated,
		BestReturn:           best.Return,
		BestScore:            best.Score,
	}, nil
}

func shouldWrite(best reasoning.Candidate, minRoundTrips int) bool {
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

	if minRoundTrips > 0 && best.Trades < minRoundTrips {
		log.TuneLog(
			"not writing candidate: trades=%d min_round_trips=%d score=%.6f",
			best.Trades,
			minRoundTrips,
			best.Score,
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
