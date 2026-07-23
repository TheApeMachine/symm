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
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ack     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
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
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ack:     make(chan struct{}, 256),
		ctx:      ctx,
		cancel:   cancel,
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
Calculate converts trade rows into typed measurements.
*/
func (signal *Signal) Calculate(
	_ []kraken.TickerData,
	trades []kraken.TradeData,
	_ []kraken.BookData,
) ([]*types.Measurement, error) {
	out := make([]*types.Measurement, 0, len(trades))

	signal.mu.Lock()
	defer signal.mu.Unlock()

	for _, row := range trades {
		measurements, err := signal.measure(row)

		if err != nil {
			return nil, err
		}

		out = append(out, measurements...)
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

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}


/*
Ack signals that one ingress frame finished Calculate so Crypto can barrier
before draining outs.
*/
func (signal *Signal) Ack() <-chan struct{} {
	return signal.ack
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			}
		}
	}()

	return out
}
