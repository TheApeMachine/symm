package hawkes

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const tickCapacity = 4096

/*
Signal detects trade-cluster excitation via a bivariate self-exciting Hawkes
model and maps the fitted state onto the thermal perspective (Frenzy /
Saturation / Organic / Exhaustion). It consumes the executed trade tape; the
per-symbol fit is cooldown-throttled inside HawkesSymbol.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map
	floor       *adaptive.SNRField
}

/*
symbolState pairs one symbol's rolling trade window with its Hawkes fitter.
*/
type symbolState struct {
	mu     sync.Mutex
	hawkes *HawkesSymbol
	ticks  []market.TradeUpdate
}

func newSymbolState() *symbolState {
	return &symbolState{hawkes: NewHawkesSymbol()}
}

func (state *symbolState) append(trade market.TradeUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.ticks) >= tickCapacity {
		state.ticks = append(state.ticks[len(state.ticks)-tickCapacity+1:], trade)
		return
	}

	state.ticks = append(state.ticks, trade)
}

func (state *symbolState) measure(now time.Time) (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.hawkes.Measure(state.ticks, now)
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
	}

	for _, channel := range []string{"trade"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(channel, 128)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	return signal
}

func (signal *Signal) Tick() error {
	for message := range signal.subscribers["trade"].Incoming {
		if message == nil || message.Value == nil {
			continue
		}

		envelope, ok := message.Value.(public.SocketMessage)

		if !ok {
			continue
		}

		rows, err := envelope.SplitDataRows()

		if err != nil {
			errnie.Error(err)

			continue
		}

		for _, row := range rows {
			trade, err := market.DecodeTrade(row)

			if err != nil {
				errnie.Error(err)

				continue
			}

			stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newSymbolState())
			state := stored.(*symbolState)
			state.append(trade)

			measurement, ok := state.measure(time.Now())

			if !ok {
				continue
			}

			measurement.Symbol = trade.Symbol
			measurement.Last = trade.Price
			measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
		}
	}

	return signal.ctx.Err()
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
