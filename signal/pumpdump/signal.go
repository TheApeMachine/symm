package pumpdump

import (
	"context"
	"time"

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
	ticker   *Ticker
	ignition *equation.Ignition
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		ticker:   NewTicker(ctx, api),
		ignition: equation.NewIgnition(),
	}
}

/*
Capture freezes the ticker journal so the ignition model consumes every unseen
state transition through one planner boundary.
*/
func (signal *Signal) Capture(at time.Time) error {
	return signal.ticker.cache.Capture(at)
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	rows, err := signal.ticker.cache.Drain(thesis.At)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"pumpdump: ticker drain failed",
			err,
		))
		return thesis
	}

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

	thesis.Signals.Store("pumpdump.tickers", rows)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
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
