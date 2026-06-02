package fluid

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
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
const rawSubscriberID = "signal/fluid:raw"

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
	floor             *adaptive.SNRField
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
		floor:       adaptive.NewSNRField(),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	signal.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	activate.Boot("signal/fluid ready")

	return signal
}

func (signal *Signal) state(symbol string) (*FluidSymbol, error) {
	if stored, ok := signal.symbols.Load(symbol); ok {
		return stored.(*FluidSymbol), nil
	}

	created, err := NewFluidSymbol(symbol)

	if err != nil {
		return nil, err
	}

	stored, _ := signal.symbols.LoadOrStore(symbol, created)

	return stored.(*FluidSymbol), nil
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TradesChannel:
				trades, err := market.DecodeTrades(&envelope)

				if err != nil {
					return fmt.Errorf("fluid: decode trades: %w", err)
				}

				for _, trade := range trades {
					state, err := signal.state(trade.Symbol)

					if err != nil {
						return fmt.Errorf("fluid: state %s: %w", trade.Symbol, err)
					}

					if err := state.FeedTradeSide(
						trade.Timestamp, trade.Qty, trade.Side,
					); err != nil {
						return fmt.Errorf("fluid: trade side %s: %w", trade.Symbol, err)
					}

					if err := signal.emit(trade.Symbol); err != nil {
						return fmt.Errorf("fluid: emit %s: %w", trade.Symbol, err)
					}
				}
			case public.TickerChannel:
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					return fmt.Errorf("fluid: decode tickers: %w", err)
				}

				for _, ticker := range tickers {
					state, err := signal.state(ticker.Symbol)

					if err != nil {
						return fmt.Errorf("fluid: state %s: %w", ticker.Symbol, err)
					}

					state.FeedTicker(ticker)

					if err := signal.emit(ticker.Symbol); err != nil {
						return fmt.Errorf("fluid: emit %s: %w", ticker.Symbol, err)
					}
				}
			case public.BookChannel:
				books, err := market.DecodeBooks(&envelope)

				if err != nil {
					return fmt.Errorf("fluid: decode books: %w", err)
				}

				for _, delta := range books {
					state, err := signal.state(delta.Symbol)

					if err != nil {
						return fmt.Errorf("fluid: state %s: %w", delta.Symbol, err)
					}

					state.FeedBook(delta)

					if err := signal.emit(delta.Symbol); err != nil {
						return fmt.Errorf("fluid: emit %s: %w", delta.Symbol, err)
					}
				}
			}
		}
	}
}

func (signal *Signal) emit(symbol string) error {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	state := raw.(*FluidSymbol)
	measurement, standout, err := state.Measure()

	if err != nil {
		return err
	}

	if measurement.Source != perspectives.SourceNone {
		if err := perspectives.AssignCategorySNR(
			&measurement, signal.floor, standout,
		); err != nil {
			return err
		}

		activate.Once("signal/fluid:measurement")
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	signal.publishField(state)

	return nil
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

	rows := make([]map[string]any, 0, 64)

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
