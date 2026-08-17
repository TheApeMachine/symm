package trader

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

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
	"github.com/theapemachine/symm/system"
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

Only the signals that own this arrival's receivers run, so a ticker epoch does
not pay for book-only conditioners. Cross-sectional cohort signals execute once
each, before any symbol-local work, in signal order: the graph solver resolves
duplicate claims by arrival, so one arrival must always produce the same
evidence sequence. Symbol-local signals then execute serially inside one
transient goroutine per symbol, which preserves symbol event order while
independent symbols progress in parallel.

The UI receives one start frame for the whole measurement cut and one
completion frame containing every produced Measurement.
*/
func (measurements *Measurements) Generate(
	thesis *types.Thesis, receivers []types.SourceType,
) error {
	thesis.Tick++
	tick := thesis.Tick

	receiverSet := make(map[types.SourceType]struct{}, len(receivers))

	for _, receiver := range receivers {
		receiverSet[receiver] = struct{}{}
	}

	symbols := make([]*types.Symbol, 0)

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := value.(*types.Symbol)

		if !ok || symbol == nil {
			return true
		}

		symbol.Tick = tick
		symbols = append(symbols, symbol)

		return true
	})

	// One deterministic cut order: sync.Map range order is unspecified, and a
	// stable sequence keeps cohort rows and completion frames replayable.
	slices.SortFunc(symbols, func(left, right *types.Symbol) int {
		return strings.Compare(left.Symbol, right.Symbol)
	})

	owners := make(map[string]*types.Symbol, len(symbols))

	for _, symbol := range symbols {
		owners[symbol.Symbol] = symbol
	}

	running := datura.NewMap()
	done := datura.NewMap()
	cohorts := make([]types.CohortSignal, 0)
	localSignals := make([]types.Signal, 0)

	for _, signal := range measurements.signals {
		if _, selected := receiverSet[signal.Type()]; !selected {
			continue
		}

		running[signal.Name()] = "running"
		done[signal.Name()] = "done"

		if cohort, crossSectional := signal.(types.CohortSignal); crossSectional {
			cohorts = append(cohorts, cohort)
			continue
		}

		localSignals = append(localSignals, signal)
	}

	utils.Publish(measurements.ui, datura.NewMap(
		"tick", datura.NewMap("count", thesis.Tick),
		"activity", running,
	))

	var measuredMu sync.Mutex
	measured := make([]*types.Measurement, 0)

	collect := func(rows []*types.Measurement) {
		measuredMu.Lock()
		defer measuredMu.Unlock()
		measured = append(measured, rows...)
	}

	for _, cohort := range cohorts {
		rows := cohort.MeasureCohort(symbols, tick)

		for _, row := range rows {
			owner, found := owners[row.Symbol]

			if !found {
				return fmt.Errorf(
					"trader: cohort row for unknown symbol %s",
					row.Symbol,
				)
			}

			owner.AppendMeasurements([]*types.Measurement{row})
		}

		collect(rows)
	}

	group, _ := errgroup.WithContext(measurements.ctx)

	for _, symbol := range symbols {
		symbol := symbol

		group.Go(func() error {
			for _, signal := range localSignals {
				rows := signal.Measure(symbol, tick)
				symbol.AppendMeasurements(rows)
				collect(rows)
			}

			return nil
		})
	}

	err := group.Wait()

	utils.Publish(measurements.ui, datura.NewMap(
		"activity", done,
		"measurements", measured,
	))

	return err
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
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"trader: failed to close signal",
				err,
			))
		}
	}

	return nil
}
