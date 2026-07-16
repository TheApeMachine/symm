package pumpdump

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal owns pump ignition measurements derived from ticker price, volume, and
spread. Book imbalance and signed trade flow remain separate depthflow and CVD
evidence so one market observation cannot masquerade as corroborating signals.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ignition *equation.Ignition
	ui       chan []byte
}

func (signal *Signal) Measure(thesis *types.Thesis) chan []*types.Measurement {
	out := make(chan []*types.Measurement)

	go func() {
		defer close(out)

		measurements, err := signal.Calculate(thesis.Market())

		if err != nil {
			errnie.Error(err)
			out <- nil
			return
		}

		out <- measurements
		signal.Publish(measurements)
	}()

	return out
}

func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		ignition: equation.NewIgnition(),
		ui:       ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	filtered := types.FilterLatest(measurements)

	if len(filtered) == 0 {
		return
	}

	select {
	case signal.ui <- datura.Map[any]{
		"measurements": filtered,
	}.Marshal():
	default:
	}
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	rows := frame.Tickers
	out := make([]*types.Measurement, 0, len(rows))

	for _, row := range rows {
		if row.Symbol == "" || row.Volume <= 0 || row.Last == nil || row.Last.Sign() <= 0 ||
			row.Bid == nil || row.Bid.Sign() <= 0 ||
			row.Ask == nil || row.Ask.Sign() <= 0 {
			continue
		}

		output, ready, maturity, err := signal.ignition.Measure(equation.IgnitionInput{
			Symbol: row.Symbol,
			Volume: row.Volume,
			Last:   row.Last.Float64(),
			Bid:    row.Bid.Float64(),
			Ask:    row.Ask.Float64(),
		})

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !ready {
			continue
		}

		measurements, err := ignitionMeasurements(
			row.Symbol, row.Timestamp, output, maturity,
			row.Bid.Float64(), row.Ask.Float64(),
		)

		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
			continue
		}

		out = append(out, measurements...)
	}

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
