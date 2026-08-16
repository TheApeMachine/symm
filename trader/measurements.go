package trader

import (
	"context"

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
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
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

	quotes := types.NewQuoteHistory(system.Cfg.PumpDump.Capacity)

	return &Measurements{
		ctx:    ctx,
		cancel: cancel,
		ui:     ui,
		signals: []types.Signal{
			correlation.NewSignal(ctx, api, ui),
			cvd.NewSignalWithQuotes(ctx, api, ui, quotes),
			depthflow.NewSignal(ctx, api, instrument, ui),
			exhaust.NewSignal(ctx, api, instrument, ui),
			hawkes.NewSignal(ctx, api, ui),
			leadlag.NewSignal(ctx, api, ui),
			liquidity.NewSignal(ctx, api, ui),
			pumpdump.NewSignalWithQuotes(ctx, api, ui, quotes),
			sentiment.NewSignal(ctx, api, ui),
			toxicity.NewSignal(ctx, api, ui),
		},
	}
}

/*
Generate measures one transport arrival without turning the stream into a
resident inbox topology.

Each symbol is owned by one transient goroutine, and all symbol-local signals
execute serially inside it. That preserves symbol event order while still
allowing independent symbols to progress in parallel. Cross-sectional signals
execute once each and receive the complete thesis universe.

The UI receives one start frame for the whole measurement cut and one completion
frame containing every produced Measurement. Signal activity is a map inside
those frames, rather than a stream of tiny running/done messages.
*/
func (measurements *Measurements) Generate(
	thesis *types.Thesis, receivers []types.SourceType,
) error {
	thesis.Tick++
	tick := thesis.Tick

	group, _ := errgroup.WithContext(measurements.ctx)

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		symbol.Tick = tick

		group.Go(func() error {
			for _, signal := range measurements.signals {
				symbol.AppendMeasurements(signal.Measure(symbol))
			}

			return nil
		})

		return true
	})

	return group.Wait()
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
		if signal == nil {
			continue
		}

		if err := signal.Close(); err != nil {
			return err
		}
	}

	return nil
}
