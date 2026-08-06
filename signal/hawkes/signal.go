package hawkes

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken/websocket"
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
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	normalize     normalizer
	sample        *excitation.Sample
	process       *excitation.Process
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
}

/*
NewSignal constructs the symbol-local excitation measurement pipeline. Its
trade component is the sole owner of the mutable marked-arrival history.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		sample:        excitation.NewSample(),
		process:       excitation.NewProcess(),
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

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
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements, true)
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	trades := thesis.MarketTrades()

	if len(trades) == 0 {
		return nil
	}

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	for _, row := range trades {
		input, ready, err := signal.sample.MeasureArrival(excitation.TradeInput{
			Symbol:    row.Symbol,
			Side:      row.Side,
			Timestamp: row.Timestamp,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))

			continue
		}

		if !ready {
			continue
		}

		output, ready, err := signal.process.Measure(input)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))

			continue
		}

		if !ready {
			continue
		}

		rowMeasurements := signal.measurements(row.Symbol, output)
		measurements = append(measurements, rowMeasurements...)

		if row.Symbol == types.Focus() {
			out = append(out, rowMeasurements...)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

/*
Close releases the receiver's owned resources.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
