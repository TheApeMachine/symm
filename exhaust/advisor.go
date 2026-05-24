package exhaust

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
)

const exhaustSource = "exhaust"

/*
Exhaust models book thinning and momentum exhaustion for open-position exits.
It is not an entry signal; it advises the portfolio when to close early.
*/
type Exhaust struct {
	ctx     context.Context
	cancel  context.CancelFunc
	market  engine.MarketReader
	history *historyStore
	watch   *engine.SymbolWatch
	mu      sync.RWMutex
	symbols map[string]struct{}
}

var _ engine.Ticker = (*Exhaust)(nil)

/*
NewExhaust wires market snapshots into an exit urgency advisor.
*/
func NewExhaust(
	ctx context.Context,
	marketRelay engine.MarketReader,
	watch *engine.SymbolWatch,
) (*Exhaust, error) {
	ctx, cancel := context.WithCancel(ctx)

	exhaust := &Exhaust{
		ctx:     ctx,
		cancel:  cancel,
		market:  marketRelay,
		history: newHistoryStore(),
		watch:   watch,
		symbols: make(map[string]struct{}),
	}

	return exhaust, errnie.Require(map[string]any{
		"ctx":    ctx,
		"market": marketRelay,
	})
}

/*
Source identifies this advisor in telemetry.
*/
func (exhaust *Exhaust) Source() string {
	return exhaustSource
}

/*
WatchSymbol tracks one open position for exit microstructure updates.
*/
func (exhaust *Exhaust) WatchSymbol(symbol string) {
	if symbol == "" {
		return
	}

	exhaust.mu.Lock()
	exhaust.symbols[symbol] = struct{}{}
	exhaust.mu.Unlock()
}

/*
ForgetSymbol stops tracking one closed position.
*/
func (exhaust *Exhaust) ForgetSymbol(symbol string) {
	if symbol == "" {
		return
	}

	exhaust.mu.Lock()
	delete(exhaust.symbols, symbol)
	exhaust.mu.Unlock()
}

/*
Tick samples watched symbols from the market relay cache.
*/
func (exhaust *Exhaust) Tick() bool {
	if exhaust.market == nil {
		return false
	}

	symbols := exhaust.watchedSymbols()

	if len(symbols) == 0 {
		return false
	}

	for _, symbol := range symbols {
		snapshot := exhaust.market.Read(symbol)
		exhaust.history.observe(
			symbol,
			bidDepth(snapshot.BidLevels),
			askDepth(snapshot.AskLevels),
			snapshot.Density,
			snapshot.SpreadBPS,
			snapshot.BuyPressure,
			snapshot.Imbalance,
			snapshot.Last,
		)
	}

	return false
}

/*
ExitUrgency returns urgency in [0, 1] and a reason label for one open position.
*/
func (exhaust *Exhaust) ExitUrgency(symbol string, side int) (float64, string) {
	history, ok := exhaust.history.snapshot(symbol)

	if !ok {
		return 0, ""
	}

	if side < 0 {
		return exitScoreShort(history)
	}

	return exitScoreLong(history)
}

func (exhaust *Exhaust) watchedSymbols() []string {
	exhaust.mu.RLock()
	defer exhaust.mu.RUnlock()

	symbols := make([]string, 0, len(exhaust.symbols))

	for symbol := range exhaust.symbols {
		symbols = append(symbols, symbol)
	}

	return symbols
}

func bidDepth(levels []market.BookLevel) float64 {
	total := 0.0

	for _, level := range levels {
		if level.Volume > 0 {
			total += level.Volume
		}
	}

	return total
}

func askDepth(levels []market.BookLevel) float64 {
	return bidDepth(levels)
}
