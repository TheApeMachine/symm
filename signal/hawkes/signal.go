package hawkes

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	signalpool "github.com/theapemachine/symm/signal"
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
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	broadcasts   map[string]*qpool.BroadcastGroup
	subscribers  map[string]*qpool.Subscriber
	symbols      sync.Map
	tradeScratch []tradeTouch
	floor        *adaptive.SNRField
}

type tradeTouch struct {
	symbol string
	state  *symbolState
	last   float64
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
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/hawkes ready", "signal/hawkes")

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

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			channel, _ := envelope["channel"].(string)
			rawData, _ := envelope["data"].(json.RawMessage)
			sm := &public.SocketMessage{Channel: channel, Data: rawData}

			switch channel {
			case public.TradesChannel:
				trades := signalpool.GetTrades(sm)

				if err := signal.observeTrades(trades); err != nil {
					errnie.Error(err, "hawkes: observe trades")
				}
			}
		}
	}
}

func (signal *Signal) observeTrades(trades []market.TradeUpdate) error {
	touches := signal.tradeScratch[:0]
	indexBySymbol := make(map[string]int, len(trades))

	for _, trade := range trades {
		if trade.Price <= 0 || trade.Qty <= 0 {
			continue
		}

		stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newSymbolState())
		state := stored.(*symbolState)
		state.append(trade)

		if touchIndex, ok := indexBySymbol[trade.Symbol]; ok {
			touches[touchIndex].last = trade.Price
			continue
		}

		indexBySymbol[trade.Symbol] = len(touches)
		touches = append(touches, tradeTouch{
			symbol: trade.Symbol,
			state:  state,
			last:   trade.Price,
		})
	}

	signal.tradeScratch = touches

	if len(touches) == 0 {
		return nil
	}

	return signal.publishTouches(touches)
}

func (signal *Signal) publishTouches(touches []tradeTouch) error {
	tasks := make([]chan *qpool.QValue[any], 0, len(touches))

	for _, touch := range touches {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			now := touchLastTime(touch.state)
			measurement, standout, err := touch.state.measure(now)

			if err != nil {
				return nil, err
			}

			if measurement.Source == perspectives.SourceNone {
				return nil, nil
			}

			measurement.Symbol = touch.symbol
			measurement.Last = touch.last

			if err := perspectives.AssignCategorySNR(
				&measurement, signal.floor, standout,
			); err != nil {
				return nil, err
			}

			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

			if ui := signal.broadcasts["ui"]; ui != nil {
				ui.Send(&qpool.QValue[any]{
					Value: map[string]any{
						"chart":      "gauge",
						"source":     measurement.Source.String(),
						"confidence": measurement.Confidence,
						"snr":        measurement.SNR,
					},
				})
			}

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)
	}

	return err
}

func touchLastTime(state *symbolState) time.Time {
	state.mu.Lock()
	defer state.mu.Unlock()

	if tickCount := len(state.ticks); tickCount > 0 {
		return state.ticks[tickCount-1].Timestamp
	}

	return time.Now()
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
