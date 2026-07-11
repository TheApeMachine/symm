package trader

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

/*
universeRanker scores every symbol observed in the ticker tier and orders
them best-first using an unweighted Borda count across quote-notional
liquidity, executable top-of-book depth, and executable round-trip cost.
Rank-summing lets axes of incompatible units (quote currency, base
quantity, basis points) combine into one ordering without hand-picked
weights between them.
*/
type universeRanker struct {
	price       *broker.Price
	maxQuoteAge time.Duration
}

func newUniverseRanker(price *broker.Price, maxQuoteAge time.Duration) *universeRanker {
	return &universeRanker{price: price, maxQuoteAge: maxQuoteAge}
}

type universeCandidate struct {
	symbol    string
	liquidity float64
	depth     float64
	cost      float64
}

/*
rank returns every eligible symbol in snapshot ordered best-first. A
symbol is ineligible when its quote is stale or non-executable, or when no
current round-trip friction (spread plus taker fees) can be derived for it.
*/
func (ranker *universeRanker) rank(snapshot map[string]kraken.TickerData) []string {
	candidates := make([]universeCandidate, 0, len(snapshot))

	for symbol, row := range snapshot {
		candidate, ok := ranker.score(symbol, row)

		if !ok {
			continue
		}

		candidates = append(candidates, candidate)
	}

	return ranker.order(candidates)
}

func (ranker *universeRanker) score(symbol string, row kraken.TickerData) (universeCandidate, bool) {
	if row.Timestamp.IsZero() || time.Since(row.Timestamp) > ranker.maxQuoteAge {
		return universeCandidate{}, false
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask < bid {
		return universeCandidate{}, false
	}

	notionalRate := row.Vwap
	if notionalRate <= 0 {
		notionalRate = row.Last.Float64()
	}

	if notionalRate <= 0 || row.Volume <= 0 {
		return universeCandidate{}, false
	}

	depthQty := min(row.BidQty, row.AskQty)

	if depthQty <= 0 {
		return universeCandidate{}, false
	}

	friction, ok := ranker.price.RoundTripFriction(symbol)

	if !ok {
		return universeCandidate{}, false
	}

	return universeCandidate{
		symbol:    symbol,
		liquidity: row.Volume * notionalRate,
		depth:     depthQty * (bid + ask) / 2,
		cost:      friction.Float64(),
	}, true
}

func (ranker *universeRanker) order(candidates []universeCandidate) []string {
	if len(candidates) == 0 {
		return nil
	}

	liquidityRank := rankBy(candidates, true, func(candidate universeCandidate) float64 {
		return candidate.liquidity
	})
	depthRank := rankBy(candidates, true, func(candidate universeCandidate) float64 {
		return candidate.depth
	})
	costRank := rankBy(candidates, false, func(candidate universeCandidate) float64 {
		return candidate.cost
	})

	type totalRank struct {
		symbol string
		sum    int
	}

	totals := make([]totalRank, len(candidates))

	for index, candidate := range candidates {
		totals[index] = totalRank{
			symbol: candidate.symbol,
			sum:    liquidityRank[index] + depthRank[index] + costRank[index],
		}
	}

	sort.SliceStable(totals, func(i, j int) bool {
		if totals[i].sum != totals[j].sum {
			return totals[i].sum < totals[j].sum
		}

		return totals[i].symbol < totals[j].symbol
	})

	ordered := make([]string, len(totals))

	for index, entry := range totals {
		ordered[index] = entry.symbol
	}

	return ordered
}

/*
rankBy returns each candidate's 0-indexed rank under value (0 is best):
descending order when higher values are better, ascending when lower
values are better.
*/
func rankBy(
	candidates []universeCandidate,
	descending bool,
	value func(universeCandidate) float64,
) []int {
	order := make([]int, len(candidates))

	for index := range order {
		order[index] = index
	}

	sort.SliceStable(order, func(i, j int) bool {
		left, right := value(candidates[order[i]]), value(candidates[order[j]])

		if descending {
			return left > right
		}

		return left < right
	})

	ranks := make([]int, len(candidates))

	for position, index := range order {
		ranks[index] = position
	}

	return ranks
}
