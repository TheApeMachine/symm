package trader

import (
	"context"
	"fmt"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

type Measurements struct {
	ctx     context.Context
	cancel  context.CancelFunc
	signals []types.Signal
	ui      chan []byte
}

func NewMeasurements(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	ui chan []byte,
) *Measurements {
	ctx, cancel := context.WithCancel(ctx)

	return &Measurements{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
		signals: []types.Signal{
			correlation.NewSignal(ctx, api, ui),
			cvd.NewSignal(ctx, api, ui),
			depthflow.NewSignal(ctx, api, instrument, ui),
			exhaust.NewSignal(ctx, api, instrument, ui),
			hawkes.NewSignal(ctx, api, ui),
			leadlag.NewSignal(ctx, api, ui),
			liquidity.NewSignal(ctx, api, ui),
			pumpdump.NewSignal(ctx, api, ui),
			sentiment.NewSignal(ctx, api, ui),
			toxicity.NewSignal(ctx, api, ui),
		},
	}
}

/*
Update takes the raw market data and runs it through all the signals to turn
them into measurements. This is a "conditioning" stage that prepares the data
for the logic stage, which can be seen as an "enrichment" process.
*/
func (measurements *Measurements) Update(thesis *types.Thesis) error {
	if measurements == nil || thesis == nil {
		return errnie.Err(
			errnie.Validation,
			"measurements: conditioner and thesis required",
			nil,
		)
	}

	group, _ := errgroup.WithContext(measurements.ctx)

	for _, signal := range measurements.signals {
		group.Go(func() error {
			symbolGroup, _ := errgroup.WithContext(measurements.ctx)

			thesis.Symbols.Range(func(key, value any) bool {
				symbolGroup.Go(func() error {
					symbol, ok := value.(*types.Symbol)

					if !ok {
						return errnie.Err(
							errnie.Internal,
							"measurements: symbol type assertion failed",
							nil,
						)
					}

					focused := make([]*types.Measurement, 0)

					for _, measurement := range signal.Measure(symbol) {
						if measurement == nil {
							continue
						}

						thesis.Symbol(measurement.Symbol).AppendMeasurement(
							signal.Type(), measurement,
						)

						if measurement.Symbol == types.Focus() {
							focused = append(focused, measurement)
						}
					}

					if len(focused) > 0 {
						utils.Publish(measurements.ui, datura.NewMap(
							"measurements", focused,
						))
					}

					return nil
				})

				return true
			})

			if err := symbolGroup.Wait(); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf(
						"measurements: signal failure [%s]",
						err.Error(),
					),
					err,
				))
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf(
				"measurements: signal failure [%s]",
				err.Error(),
			),
			err,
		))
	}

	return nil
}

/*
Close releases every signal conditioner owned by the measurement stage.
*/
func (measurements *Measurements) Close() error {
	if measurements == nil {
		return nil
	}

	measurements.cancel()

	for _, signal := range measurements.signals {
		if err := signal.Close(); err != nil {
			return err
		}
	}

	return nil
}
