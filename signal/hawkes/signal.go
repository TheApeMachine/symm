package hawkes

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal measures the buy/sell trade-arrival process as

	λ(t) = μ + Σ A exp(-β(t-ti)).

It emits empirical rates before the model is identifiable, then fitted μ, λ,
A, β, spectral stability, offspring expectations, and restricted likelihood
comparisons. These are statistical measurements rather than market regimes;
forecast readiness remains false until residual and out-of-sample validation
exists.
*/
type Signal struct {
	status        types.Status
	thesis        *types.Thesis
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	normalize     normalizer
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:  types.INITIALIZING,
		thesis:  planner.Thesis,
		ctx:     ctx,
		cancel:  cancel,
		api:     api,
		planner: planner,
		ui:      ui,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe("ticker", types.NewSubscription[any]()),
			"trade":  api.Subscribe("trade", types.NewSubscription[any]()),
		},
		subscribers: &sync.Map{},
	}
	signal.thesis.Causal.Store("signal:hawkes:sample", excitation.NewSample())
	signal.thesis.Causal.Store("signal:hawkes:process", excitation.NewProcess())
	signal.thesis.Causal.Store("signal:hawkes:mu", &sync.Mutex{})

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return signalshared.Subscribe(
		&signal.mu,
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) publishThesis() {
	if signal.subscribers == nil {
		return
	}

	subscribers, ok := signal.subscribers.Load("thesis")

	if ok && subscribers != nil {
		for _, subscriber := range subscribers.([]*types.Subscription[any]) {
			subscriber.Send(signal.thesis)
		}
	}
}

/*
onTicker treats the ticker stream as Hawkes cadence only so the signal can emit a
shared cut after warmed trade arrivals without pretending ticker rows carry
microstructure measurements.
*/
func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.thesis.AppendMeasurements(
		types.SourceHawkes,
		signal.Calculate(ticker.Data, nil, nil),
		types.Stamp{At: time.Now(), Entity: types.MarketTicker},
	)

	signal.publishThesis()
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case ticker := <-signal.subscriptions["ticker"].Channel:
				if ticker, ok := ticker.(*kraken.Ticker); ok {
					signal.onTicker(ticker)
				}
			case trade := <-signal.subscriptions["trade"].Channel:
				if trade, ok := trade.(*kraken.Trade); ok {
					signal.onTrade(trade)
				}
			}
		}
	}()
}

func (signal *Signal) onTrade(trade *kraken.Trade) {
	signal.thesis.AppendMeasurements(
		types.SourceHawkes,
		signal.Calculate(nil, trade.Data, nil),
		types.Stamp{At: time.Now(), Entity: types.MarketTrade},
	)

	signal.publishThesis()
}

/*
Outcome returns the latest measured Hawkes outcome for one symbol.
*/
func (signal *Signal) Outcome(symbol string) (excitation.Outcome, bool) {
	if signal == nil || signal.thesis == nil {
		return excitation.Outcome{}, false
	}

	found, _ := signal.thesis.Causal.Load("signal:hawkes:mu")
	mu := found.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	found, _ = signal.thesis.Causal.Load("signal:hawkes:process")
	return found.(*excitation.Process).Outcome(symbol)
}

/*
Symbols returns every symbol with retained Hawkes excitation state.
*/
func (signal *Signal) Symbols() []string {
	if signal == nil || signal.thesis == nil {
		return nil
	}

	found, _ := signal.thesis.Causal.Load("signal:hawkes:mu")
	mu := found.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	found, _ = signal.thesis.Causal.Load("signal:hawkes:process")
	return found.(*excitation.Process).Symbols()
}

/*
Calculate converts executed trades into Hawkes measurements because arrivals on
the public trade tape are the authoritative event stream for self-excitation.
*/
func (signal *Signal) Calculate(
	_ []kraken.TickerData,
	trades []kraken.TradeData,
	_ []kraken.BookData,
) []*types.Measurement {
	out := make([]*types.Measurement, 0, len(trades))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	found, _ := signal.thesis.Causal.Load("signal:hawkes:mu")
	mu := found.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	for _, row := range trades {
		measurements, err := signal.measure(row)

		if err != nil {
			errnie.Error(err)
			return nil
		}

		out = append(out, measurements...)

		if row.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurements...,
			)
		}
	}

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
}

/*
measure advances both the arrival sampler and Hawkes process for one trade so the
signal only emits once the excitation state is numerically ready.
*/
func (signal *Signal) measure(row kraken.TradeData) ([]*types.Measurement, error) {
	found, _ := signal.thesis.Causal.Load("signal:hawkes:sample")
	input, ready, err := found.(*excitation.Sample).MeasureArrival(excitation.TradeInput{
		Symbol:    row.Symbol,
		Side:      row.Side,
		Timestamp: row.Timestamp,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	found, _ = signal.thesis.Causal.Load("signal:hawkes:process")
	output, ready, err := found.(*excitation.Process).Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	return signal.measurements(row.Symbol, output), nil
}

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
