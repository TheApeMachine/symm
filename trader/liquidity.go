package trader

import (
	"fmt"
	"math"
	"sort"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type liquidityCandidate struct {
	symbol   string
	notional float64
	depth    float64
	spread   float64
	ranks    [3]float64
}

type liquidityValue struct {
	symbol string
	value  float64
}

/*
LiquidityRanker selects symbols whose weakest liquidity dimension is strongest.
It uses percentile ranks for quote notional, executable depth, and spread so
the three differently-scaled quantities are comparable without weights.
*/
type LiquidityRanker struct {
	candidates []liquidityCandidate
}

/*
NewLiquidityRanker validates ticker liquidity inputs and derives candidates.
*/
func NewLiquidityRanker(tickers []kraken.TickerData) (*LiquidityRanker, error) {
	ranker := &LiquidityRanker{
		candidates: make([]liquidityCandidate, 0, len(tickers)),
	}

	for _, ticker := range tickers {
		if err := ranker.append(ticker); err != nil {
			return nil, err
		}
	}

	return ranker, nil
}

func (ranker *LiquidityRanker) append(ticker kraken.TickerData) error {
	if ticker.Symbol == "" || ticker.Bid == nil || ticker.Ask == nil {
		return fmt.Errorf("trader liquidity: complete quote required for %s", ticker.Symbol)
	}

	if ticker.Vwap <= 0 && ticker.Last == nil {
		return fmt.Errorf("trader liquidity: traded rate required for %s", ticker.Symbol)
	}

	bid := ticker.Bid.Float64()
	ask := ticker.Ask.Float64()

	if bid <= 0 || ask < bid {
		return fmt.Errorf("trader liquidity: valid two-sided quote required for %s", ticker.Symbol)
	}

	notional := types.QuoteNotional(ticker)
	depth := types.ExecutableDepth(ticker)
	spread := (ask - bid) / ((ask + bid) / 2)

	if invalidLiquidity(notional) || invalidLiquidity(depth) || invalidSpread(spread) {
		return fmt.Errorf("trader liquidity: positive finite inputs required for %s", ticker.Symbol)
	}

	ranker.candidates = append(ranker.candidates, liquidityCandidate{
		symbol:   ticker.Symbol,
		notional: notional,
		depth:    depth,
		spread:   spread,
	})

	return nil
}

/*
Rank returns the maximin liquidity order, using leximin tie-breaking across
the remaining percentile dimensions and symbol identity as the final tie.
*/
func (ranker *LiquidityRanker) Rank(limit int) []string {
	ranker.score()

	sort.Slice(ranker.candidates, func(left, right int) bool {
		for dimension := range ranker.candidates[left].ranks {
			if ranker.candidates[left].ranks[dimension] == ranker.candidates[right].ranks[dimension] {
				continue
			}

			return ranker.candidates[left].ranks[dimension] > ranker.candidates[right].ranks[dimension]
		}

		return ranker.candidates[left].symbol < ranker.candidates[right].symbol
	})

	symbols := make([]string, min(limit, len(ranker.candidates)))

	for index := range symbols {
		symbols[index] = ranker.candidates[index].symbol
	}

	return symbols
}

func (ranker *LiquidityRanker) score() {
	notionals := make([]liquidityValue, len(ranker.candidates))
	depths := make([]liquidityValue, len(ranker.candidates))
	spreads := make([]liquidityValue, len(ranker.candidates))

	for index, candidate := range ranker.candidates {
		notionals[index] = liquidityValue{candidate.symbol, candidate.notional}
		depths[index] = liquidityValue{candidate.symbol, candidate.depth}
		spreads[index] = liquidityValue{candidate.symbol, candidate.spread}
	}

	notionalRanks := ranker.percentiles(notionals, true)
	depthRanks := ranker.percentiles(depths, true)
	spreadRanks := ranker.percentiles(spreads, false)

	for index := range ranker.candidates {
		candidate := &ranker.candidates[index]
		candidate.ranks = [3]float64{
			notionalRanks[candidate.symbol],
			depthRanks[candidate.symbol],
			spreadRanks[candidate.symbol],
		}
		sort.Float64s(candidate.ranks[:])
	}
}

func (ranker *LiquidityRanker) percentiles(
	values []liquidityValue,
	higherIsBetter bool,
) map[string]float64 {
	sort.Slice(values, func(left, right int) bool {
		if values[left].value == values[right].value {
			return values[left].symbol < values[right].symbol
		}

		if higherIsBetter {
			return values[left].value > values[right].value
		}

		return values[left].value < values[right].value
	})

	ranks := make(map[string]float64, len(values))

	if len(values) == 1 {
		ranks[values[0].symbol] = 1
		return ranks
	}

	for start := 0; start < len(values); {
		end := start + 1

		for end < len(values) && values[end].value == values[start].value {
			end++
		}

		averagePosition := float64(start+end-1) / 2
		percentile := 1 - averagePosition/float64(len(values)-1)

		for index := start; index < end; index++ {
			ranks[values[index].symbol] = percentile
		}

		start = end
	}

	return ranks
}

func invalidLiquidity(value float64) bool {
	return value <= 0 || math.IsNaN(value) || math.IsInf(value, 0)
}

func invalidSpread(value float64) bool {
	return value < 0 || math.IsNaN(value) || math.IsInf(value, 0)
}
