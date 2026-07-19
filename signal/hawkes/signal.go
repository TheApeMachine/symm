package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
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
	ctx      context.Context
	cancel   context.CancelFunc
	api      *websocket.API
	sample   *excitation.Sample
	process  *excitation.Process
	evidence *Evidence
	ui       chan []byte
	mu       sync.Mutex
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires the public trade tape; Hawkes measures the marked buy/sell
arrival process from trades alone.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamTrade
}

/*
Measure supports direct replay against the legacy signal-local trade journal.
The live runtime uses Calculate with the central immutable market cut.
*/
func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:      ctx,
		cancel:   cancel,
		api:      api,
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
		ui:       ui,
	}

	return signal
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
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	rows := frame.Trades
	out := make([]*types.Measurement, 0, len(rows))

	signal.mu.Lock()

	for _, row := range rows {
		measurements, err := signal.measure(row)

		if err != nil {
			errnie.Error(err)
			continue
		}

		out = append(out, measurements...)
	}

	signal.mu.Unlock()
	return out, nil
}

/*
measure updates the marked arrival stream and emits every numerical quantity
supported by the estimator's current readiness level.
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
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
