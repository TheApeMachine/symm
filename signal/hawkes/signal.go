package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
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
	*types.Actor
	thesis    *types.Thesis
	ctx       context.Context
	cancel    context.CancelFunc
	sample    *excitation.Sample
	process   *excitation.Process
	normalize normalizer
	ui        chan []byte
	mu        sync.Mutex
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		sample:  excitation.NewSample(),
		process: excitation.NewProcess(),
		ui:      ui,
	}

	signal.Actor = types.NewActor(ctx, "hawkes", map[string]types.Handler{
		"ticker": {
			Topic: "ticker",
			Fn:    signal.onTicker,
		},
		"trade": {
			Topic: "trade",
			Fn:    signal.onTrade,
		},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceHawkes)
}

/*
Initialize wires ticker and trade ingress from Live. Cut serialisation stays on
the Coordinator→Analyzer depth-one edge; Hawkes itself keeps a normal buffer so
a busy cascade cannot stall Live's trade root and starve cadence.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

/*
onTicker treats the ticker stream as Hawkes cadence only so the signal can emit a
shared cut after warmed trade arrivals without pretending ticker rows carry
microstructure measurements.
*/
func (signal *Signal) onTicker(message any) any {
	_ = message.(*kraken.Ticker)
	cut := signal.cut()

	if len(cut.Symbols()) == 0 {
		return nil
	}

	return cut
}

func (signal *Signal) onTrade(message any) any {
	return signal.thesis.AppendMeasuremnts(
		types.SourceHawkes,
		signal.Calculate(nil, message.(*kraken.Trade).Data, nil),
	)
}

/*
Outcome returns the latest measured Hawkes outcome for one symbol.
*/
func (signal *Signal) Outcome(symbol string) (excitation.Outcome, bool) {
	if signal == nil || signal.process == nil {
		return excitation.Outcome{}, false
	}

	signal.mu.Lock()
	defer signal.mu.Unlock()

	return signal.process.Outcome(symbol)
}

/*
Symbols returns every symbol with retained Hawkes excitation state.
*/
func (signal *Signal) Symbols() []string {
	if signal == nil || signal.process == nil {
		return nil
	}

	signal.mu.Lock()
	defer signal.mu.Unlock()

	return signal.process.Symbols()
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

	signal.mu.Lock()
	defer signal.mu.Unlock()

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
	input, ready, err := signal.sample.MeasureArrival(excitation.TradeInput{
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

	output, ready, err := signal.process.Measure(input)

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
