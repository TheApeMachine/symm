package fluid

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

/*
Signal applies order-book fluid dynamics per symbol and maps the field onto the
mechanical perspective (Laminar / Turbulent / Inertial / Viscous). It consumes
book, trades, and ticks; the field model lives in FluidSymbol.
*/
const rawSubscriberID = "signal/fluid:raw"

type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	symbols      sync.Map
	fieldScratch []*FluidSymbol
	ui           *qpool.BroadcastGroup
	floor        *adaptive.SNRField
	rawDump      *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		floor:       adaptive.NewSNRField(),
		rawDump:     rawdump.Open("fluid"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = bus.Group(pool, "measurements", 10*time.Millisecond)

	signal.ui = bus.Group(pool, "ui", 10*time.Millisecond)

	errnie.Info("signal/fluid ready", "signal/fluid")

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

			sm, ok := signalpool.SocketMessageFromValue(message.Value)

			if !ok {
				continue
			}

			switch sm.Channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)

				for _, trade := range trades {
					state, err := signal.state(trade.Symbol)

					if err != nil {
						errnie.Error(err, "fluid: state %s", trade.Symbol)
						continue
					}

					if err := state.FeedTradeSide(
						trade.Timestamp, trade.Qty, trade.Side,
					); err != nil {
						errnie.Error(err, "fluid: trade side %s", trade.Symbol)
						continue
					}

					if err := signal.emit(trade.Symbol); err != nil {
						errnie.Error(err, "fluid: emit %s", trade.Symbol)
						continue
					}
				}
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)

				for _, ticker := range tickers {
					state, err := signal.state(ticker.Symbol)

					if err != nil {
						errnie.Error(err, "fluid: state %s", ticker.Symbol)
						continue
					}

					state.FeedTicker(ticker)

					if err := signal.emit(ticker.Symbol); err != nil {
						errnie.Error(err, "fluid: emit %s", ticker.Symbol)
						continue
					}
				}
			case public.BookChannel:
				books := signalpool.GetBooks(sm)

				for _, delta := range books {
					state, err := signal.state(delta.Symbol)

					if err != nil {
						errnie.Error(err, "fluid: state %s", delta.Symbol)
						continue
					}

					state.FeedBook(delta)

					if err := signal.emit(delta.Symbol); err != nil {
						errnie.Error(err, "fluid: emit %s", delta.Symbol)
						continue
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

		if err := signal.rawDump.Write(rawRecord{
			TimestampUnixNano: time.Now().UTC().UnixNano(),
			Symbol:            measurement.Symbol,
			Category:          measurement.Category,
			Strength:          measurement.Strength,
			Confidence:        measurement.Confidence,
			SNR:               measurement.SNR,
			Standout:          standout,
			Last:              measurement.Last,
			SpreadBPS:         measurement.SpreadBPS,
		}); err != nil {
			return err
		}

		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

		if signal.ui != nil {
			signal.ui.Send(&qpool.QValue[any]{
				Value: map[string]any{
					"chart":      "gauge",
					"source":     measurement.Source.String(),
					"confidence": measurement.Confidence,
					"snr":        measurement.SNR,
				},
			})
		}
	}

	if err := signal.publishField(state); err != nil {
		return err
	}

	return nil
}

// publishField ships an aggregated universe field snapshot to the dashboard
// surface when at least one symbol has field data.
func (signal *Signal) publishField(_ *FluidSymbol) error {
	states := signal.fieldScratch[:0]

	signal.symbols.Range(func(_, value any) bool {
		states = append(states, value.(*FluidSymbol))

		return true
	})

	signal.fieldScratch = states

	tasks := make([]chan *qpool.QValue[any], 0, len(states))

	for _, fluidState := range states {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			row := fluidState.Row()

			if row == nil {
				return nil, nil
			}

			return row, nil
		}))
	}

	rows := make([]map[string]any, 0, len(tasks))
	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)

		if value.Value == nil {
			continue
		}

		row, ok := value.Value.(map[string]any)

		if !ok {
			return errors.Join(err, errors.New("fluid: field row has unexpected type"))
		}

		rows = append(rows, row)
	}

	if err != nil {
		return err
	}

	if len(rows) == 0 {
		return nil
	}

	signal.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"type":         "fluid",
		"ts":           time.Now().UTC().Format(time.RFC3339Nano),
		"symbol_count": len(rows),
		"symbols":      rows,
	}})

	return nil
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
