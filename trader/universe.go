package trader

import (
	"sort"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
)

/*
Universe ranks every pair observed in the lightweight ticker/OHLC
observation tier and admits only a bounded, top-ranked, executable subset
into the heavier trade/book/level3 trading tier. Promotion favors
quote-notional liquidity, executable top-of-book depth, and low executable
round-trip cost (spread plus taker fees); any symbol the desk currently
holds is pinned regardless of rank so an open position never loses its
execution-quality feeds mid-trade. Demotion releases the trade/book/level3
subscriptions and the locally reconstructed book state for symbols that
fall out of contention.
*/
type Universe struct {
	instrument *Instrument
	desk       *broker.Desk
	orderBook  *OrderBook
	level3Book *Level3Book
	ranker     *universeRanker
	tierSize   int
	promoted   map[string]struct{}
}

/*
NewUniverse wires the two-stage universe controller around the shared
instrument catalog, live pricing, desk holdings, and the reconstructed
L2 and L3 book state that must be evicted on demotion.
*/
func NewUniverse(
	instrument *Instrument,
	price *broker.Price,
	desk *broker.Desk,
	orderBook *OrderBook,
	level3Book *Level3Book,
) *Universe {
	return &Universe{
		instrument: instrument,
		desk:       desk,
		orderBook:  orderBook,
		level3Book: level3Book,
		ranker:     newUniverseRanker(price, viper.GetDuration("trading.max_quote_age")),
		tierSize:   viper.GetInt("market.universe.trading_tier_size"),
		promoted:   map[string]struct{}{},
	}
}

/*
Promoted returns the currently admitted trading-tier symbols, sorted.
*/
func (universe *Universe) Promoted() []string {
	symbols := make([]string, 0, len(universe.promoted))

	for symbol := range universe.promoted {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)
	return symbols
}

/*
Rebalance scores every symbol in snapshot, promotes the top-ranked bounded
subset -- plus every symbol the desk currently holds -- into the trading
tier, and demotes everything that falls out of the resulting target set.
*/
func (universe *Universe) Rebalance(snapshot map[string]kraken.TickerData) error {
	if universe.tierSize <= 0 {
		return errnie.Err(
			errnie.Validation,
			"trader: universe trading tier size must be positive",
			nil,
		)
	}

	target := map[string]struct{}{}

	for symbol := range universe.desk.Holdings() {
		target[symbol] = struct{}{}
	}

	for _, symbol := range universe.ranker.rank(snapshot) {
		if len(target) >= universe.tierSize {
			break
		}

		target[symbol] = struct{}{}
	}

	return universe.apply(target)
}

func (universe *Universe) apply(target map[string]struct{}) error {
	promote := make([]string, 0)

	for symbol := range target {
		if _, already := universe.promoted[symbol]; !already {
			promote = append(promote, symbol)
		}
	}

	demote := make([]string, 0)

	for symbol := range universe.promoted {
		if _, stillTarget := target[symbol]; !stillTarget {
			demote = append(demote, symbol)
		}
	}

	sort.Strings(promote)
	sort.Strings(demote)

	if err := universe.instrument.Promote(promote); err != nil {
		return err
	}

	if err := universe.instrument.Demote(demote); err != nil {
		return err
	}

	for _, symbol := range demote {
		universe.orderBook.Reset(symbol)
		universe.level3Book.Reset(symbol)
	}

	universe.promoted = target
	return nil
}
