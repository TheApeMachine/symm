package fluid

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Signal applies order-book fluid dynamics per symbol and maps the field onto the
mechanical perspective (Laminar / Turbulent / Inertial / Viscous). It consumes
book, trades, and ticks; the field model lives in FluidSymbol.
*/
// fieldInterval rate-limits each symbol's field row to the surface. The frontend
// rebuilds the whole 32x32 grid on every row, so streaming one per book delta
// (across the full universe) burns CPU on both ends for updates the eye cannot
// resolve; a few per second per symbol keeps the surface live and cheap.
const fieldInterval = 200 * time.Millisecond

type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	ui          *qpool.BroadcastGroup
	fieldEmit   sync.Map
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)
	signal.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return signal
}

func (signal *Signal) state(symbol string) *FluidSymbol {
	stored, _ := signal.symbols.LoadOrStore(symbol, NewFluidSymbol(symbol))
	return stored.(*FluidSymbol)
}

func (signal *Signal) Tick() error {
	symbols := config.System.Symbols
	trades := market.NewTradeSubscription(signal.ctx, symbols...)
	ticks := market.NewTickerSubscription(signal.ctx, symbols...)
	books := market.NewBookSubscription(signal.ctx, config.System.BookDepthLevels, symbols...)

	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case trade, ok := <-trades:
			if !ok {
				trades = nil
				continue
			}

			if trade != nil {
				signal.state(trade.Symbol).FeedTradeSide(trade.Timestamp, trade.Qty, trade.Side)
				signal.emit(trade.Symbol)
			}
		case row, ok := <-ticks:
			if !ok {
				ticks = nil
				continue
			}

			if row != nil {
				signal.state(row.Symbol).FeedTicker(*row)
			}
		case delta, ok := <-books:
			if !ok {
				books = nil
				continue
			}

			if delta != nil {
				signal.state(delta.Symbol).FeedBook(*delta)
				signal.emit(delta.Symbol)
			}
		}
	}
}

func (signal *Signal) emit(symbol string) {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return
	}

	state := raw.(*FluidSymbol)
	measurement, ok := state.Measure()

	if ok {
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	signal.publishField(symbol, state)
}

// publishField ships the symbol's fluid-field row to the dashboard surface, rate
// limited per symbol so the universe-wide field does not flood the bus or the
// frontend grid rebuild.
func (signal *Signal) publishField(symbol string, state *FluidSymbol) {
	now := time.Now()

	if last, seen := signal.fieldEmit.Load(symbol); seen {
		if now.Sub(last.(time.Time)) < fieldInterval {
			return
		}
	}

	row := state.Row()

	if row == nil {
		return
	}

	signal.fieldEmit.Store(symbol, now)

	signal.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "field_row",
		"ts":     now.UTC().Format(time.RFC3339Nano),
		"symbol": symbol,
		"row":    row,
	}})
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
