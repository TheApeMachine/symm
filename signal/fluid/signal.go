package fluid

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Signal applies order-book fluid dynamics per symbol and maps the field onto the
mechanical perspective (Laminar / Turbulent / Inertial / Viscous). It consumes
book, trades, and ticks; the field model lives in FluidSymbol.
*/
// fieldSnapshotInterval rate-limits the aggregated universe field snapshot. The
// surface needs every symbol to build change% × vol topology; one snapshot per
// interval keeps the UI channel lean without collapsing the field to a flat
// anchor-only plane when per-pair streams are focus-gated elsewhere.
const fieldSnapshotInterval = 200 * time.Millisecond

type Signal struct {
	ctx               context.Context
	cancel            context.CancelFunc
	pool              *qpool.Q
	broadcasts        map[string]*qpool.BroadcastGroup
	subscribers       map[string]*qpool.Subscriber
	symbols           sync.Map
	tracker           *focus.Set
	ui                *qpool.BroadcastGroup
	lastFieldSnapshot atomic.Int64
}

func NewSignal(ctx context.Context, pool *qpool.Q, tracker *focus.Set) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		tracker:     tracker,
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

	signal.publishField(state)
}

// publishField ships an aggregated universe field snapshot to the dashboard
// surface. Per-pair UI streams stay focus-gated; the fluid surface is not a
// single-pair chart and needs the full symbol set to render meaningful topology.
func (signal *Signal) publishField(state *FluidSymbol) {
	if state.Row() == nil {
		return
	}

	now := time.Now()
	lastNano := signal.lastFieldSnapshot.Load()

	if lastNano > 0 && now.Sub(time.Unix(0, lastNano)) < fieldSnapshotInterval {
		return
	}

	rows := make([]map[string]any, 0, len(config.System.Symbols))

	signal.symbols.Range(func(_, value any) bool {
		row := value.(*FluidSymbol).Row()

		if row != nil {
			rows = append(rows, row)
		}

		return true
	})

	if len(rows) == 0 {
		return
	}

	signal.lastFieldSnapshot.Store(now.UnixNano())

	signal.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":        "field_snapshot",
		"ts":           now.UTC().Format(time.RFC3339Nano),
		"symbol_count": len(rows),
		"symbols":      rows,
	}})
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
