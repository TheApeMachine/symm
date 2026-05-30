package hawkes

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
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
}

/*
symbolState pairs one symbol's rolling trade window with its Hawkes fitter.
*/
type symbolState struct {
	mu     sync.Mutex
	hawkes *HawkesSymbol
	ticks  []market.TradeUpdate
	floor  *adaptive.SNR
}

func newSymbolState() *symbolState {
	return &symbolState{hawkes: NewHawkesSymbol(), floor: adaptive.NewSNR()}
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
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) Tick() error {
	for trade := range market.NewTradeSubscription(signal.ctx, config.System.Symbols...) {
		if trade == nil {
			continue
		}

		stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newSymbolState())
		state := stored.(*symbolState)
		state.append(*trade)

		measurement, ok := state.measure(time.Now())

		if !ok {
			continue
		}

		measurement.Symbol = trade.Symbol
		measurement.Last = trade.Price
		measurement = perspectives.FinalizeSNR(
			measurement,
			measurement.SNR,
			state.floor.Score,
		)
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	return signal.ctx.Err()
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
