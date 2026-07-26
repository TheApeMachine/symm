package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
	thesis   *types.Thesis
	ctx      context.Context
	cancel   context.CancelFunc
	sample   *excitation.Sample
	process  *excitation.Process
	evidence *Evidence
	ui       chan []byte
	mu       sync.Mutex
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:      ctx,
		cancel:   cancel,
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
		ui:       ui,
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

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) > 0 {
		signal.thesis.Publish(types.SourceHawkes, measurements)
	}

	// Tickers do not feed arrivals today, but they are the market pulse. Without
	// a cut here the cascade only advances on trades and starves on thin books.
	if len(signal.Symbols()) == 0 {
		return nil
	}

	return signal.cut()
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Publish(types.SourceHawkes, measurements)

	return signal.cut()
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
Calculate converts trade rows into typed measurements.
*/
func (signal *Signal) Calculate(
	_ []kraken.TickerData,
	trades []kraken.TradeData,
	_ []kraken.BookData,
) ([]*types.Measurement, error) {
	out := make([]*types.Measurement, 0, len(trades))
	uiOut := make([]*types.Measurement, 0)

	signal.mu.Lock()
	defer signal.mu.Unlock()

	for _, row := range trades {
		measurements, err := signal.measure(row)

		if err != nil {
			return nil, err
		}

		out = append(out, measurements...)

		if row.Symbol == types.Focus() {
			uiOut = append(uiOut, measurements...)
		}
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": uiOut,
	}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
	}

	return out, nil
}

/*
measure updates the marked arrival stream and emits numerical quantities.
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

	return signal.evidence.Measure(row.Symbol, output), nil
}

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
