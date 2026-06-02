package hawkes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	tickCapacity    = 4096
	rawSubscriberID = "signal/hawkes:raw"
)

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

func (state *symbolState) measure(now time.Time) (perspectives.Measurement, float64, error) {
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

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	activate.Boot("signal/hawkes ready")

	return signal
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
					return fmt.Errorf("hawkes: decode trades: %w", err)
				}

				for _, trade := range trades {
					stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newSymbolState())
					state := stored.(*symbolState)
					state.append(trade)

					measurement, standout, err := state.measure(time.Now())

					if err != nil {
						return fmt.Errorf("hawkes: measure %s: %w", trade.Symbol, err)
					}

					if measurement.Source == perspectives.SourceNone {
						continue
					}

					measurement.Symbol = trade.Symbol
					measurement.Last = trade.Price
					if err := perspectives.AssignCategorySNR(
						&measurement, signal.floor, standout,
					); err != nil {
						return fmt.Errorf("hawkes: snr %s: %w", trade.Symbol, err)
					}

					activate.Once("signal/hawkes:measurement")
					signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
				}
			}
		}
	}
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
