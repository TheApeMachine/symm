package trader

import (
	"fmt"
	"sort"

	"github.com/theapemachine/symm/kraken"
)

/*
SubscriptionPlan separates universe-wide observation feeds from the bounded
order-flow tier that carries the expensive trade, book, and level3 channels.
*/
type SubscriptionPlan struct {
	observation [][]string
	trading     [][]string
	symbols     []string
	batchSize   int
	ranked      bool
}

/*
NewSubscriptionPlan builds deterministic subscription batches.
*/
func NewSubscriptionPlan(
	pairs []kraken.InstrumentPair,
	batchSize int,
) (*SubscriptionPlan, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("trader subscription: batch size must be positive")
	}

	if len(pairs) == 0 {
		return nil, fmt.Errorf("trader subscription: instruments required")
	}

	symbols := make([]string, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))

	for _, pair := range pairs {
		if pair.Symbol == "" {
			return nil, fmt.Errorf("trader subscription: instrument symbol required")
		}

		if _, duplicate := seen[pair.Symbol]; duplicate {
			return nil, fmt.Errorf(
				"trader subscription: duplicate instrument symbol %s",
				pair.Symbol,
			)
		}

		seen[pair.Symbol] = struct{}{}
		symbols = append(symbols, pair.Symbol)
	}

	sort.Strings(symbols)

	return &SubscriptionPlan{
		observation: subscriptionBatches(symbols, batchSize),
		symbols:     symbols,
		batchSize:   batchSize,
	}, nil
}

/*
Rank derives the bounded heavy-feed tier from complete ticker coverage.
*/
func (plan *SubscriptionPlan) Rank(
	tickers []kraken.TickerData,
	tradingTierSize int,
) error {
	if plan.ranked {
		return fmt.Errorf("trader subscription: liquidity tier already ranked")
	}

	if tradingTierSize <= 0 {
		return fmt.Errorf("trader subscription: trading tier size must be positive")
	}

	ordered, err := plan.coverage(tickers)

	if err != nil {
		return err
	}

	ranker, err := NewLiquidityRanker(ordered)

	if err != nil {
		return err
	}

	symbols := ranker.Rank(min(tradingTierSize, len(plan.symbols)))
	plan.trading = subscriptionBatches(symbols, plan.batchSize)
	plan.ranked = true
	return nil
}

func (plan *SubscriptionPlan) coverage(
	tickers []kraken.TickerData,
) ([]kraken.TickerData, error) {
	expected := make(map[string]struct{}, len(plan.symbols))

	for _, symbol := range plan.symbols {
		expected[symbol] = struct{}{}
	}

	rows := make(map[string]kraken.TickerData, len(tickers))

	for _, ticker := range tickers {
		if _, ok := expected[ticker.Symbol]; !ok {
			return nil, fmt.Errorf(
				"trader subscription: unexpected ticker symbol %s",
				ticker.Symbol,
			)
		}

		if _, duplicate := rows[ticker.Symbol]; duplicate {
			return nil, fmt.Errorf(
				"trader subscription: duplicate ticker symbol %s",
				ticker.Symbol,
			)
		}

		rows[ticker.Symbol] = ticker
	}

	ordered := make([]kraken.TickerData, 0, len(plan.symbols))

	for _, symbol := range plan.symbols {
		ticker, ok := rows[symbol]

		if !ok {
			return nil, fmt.Errorf(
				"trader subscription: ticker missing for symbol %s",
				symbol,
			)
		}

		ordered = append(ordered, ticker)
	}

	return ordered, nil
}

/*
Observation returns universe-wide ticker and OHLC subscription batches.
*/
func (plan *SubscriptionPlan) Observation() [][]string {
	return plan.observation
}

/*
Trading returns bounded trade, book, and level3 subscription batches.
*/
func (plan *SubscriptionPlan) Trading() [][]string {
	return plan.trading
}

/*
Symbols returns the exact identity set required before liquidity ranking.
*/
func (plan *SubscriptionPlan) Symbols() []string {
	return append([]string(nil), plan.symbols...)
}

/*
Ranked reports whether complete ticker data has produced a trading tier.
*/
func (plan *SubscriptionPlan) Ranked() bool {
	return plan.ranked
}

func subscriptionBatches(symbols []string, batchSize int) [][]string {
	batches := make([][]string, 0, (len(symbols)+batchSize-1)/batchSize)

	for start := 0; start < len(symbols); start += batchSize {
		end := min(start+batchSize, len(symbols))
		batches = append(batches, append([]string(nil), symbols[start:end]...))
	}

	return batches
}
