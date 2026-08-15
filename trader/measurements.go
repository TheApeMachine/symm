package trader

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

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
	ctx          context.Context
	cancel       context.CancelFunc
	signals      []types.Signal
	ui           chan []byte
	clocks       *clockBank
	dispatchedAt time.Time
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

type activeMeasurementSignal struct {
	index  int
	signal types.Signal
}

type measuredRow struct {
	signalIndex int
	symbolOrder int
	rowOrder    int
	measurement *types.Measurement
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
	thesis *types.Thesis,
	receivers []types.SourceType,
) (bool, error) {
	if measurements == nil || thesis == nil {
		return false, errnie.Error(errnie.Err(
			errnie.Validation,
			"measurements: measurement stage and thesis required",
			nil,
		))
	}

	symbols, err := measurementSymbols(thesis)

	if err != nil {
		return false, err
	}

	if len(symbols) == 0 {
		return false, nil
	}

	thesis.Tick++
	tick := thesis.Tick
	symbolOrder := make(map[string]int, len(symbols))

	for index, symbol := range symbols {
		symbol.Tick = tick
		symbolOrder[symbol.Symbol] = index
	}

	active := make([]activeMeasurementSignal, 0, len(measurements.signals))
	running := datura.NewMap()
	done := datura.NewMap()

	for index, signal := range measurements.signals {
		if signal == nil || !slices.Contains(receivers, signal.Type()) {
			continue
		}

		active = append(active, activeMeasurementSignal{index: index, signal: signal})
		running[string(signal.Type())] = "running"
		done[string(signal.Type())] = "done"
	}

	startFrame := datura.NewMap(
		"tick", datura.NewMap("count", tick),
	)

	utils.Publish(measurements.ui, startFrame)

	if len(active) == 0 {
		return false, nil
	}

	group, _ := errgroup.WithContext(measurements.ctx)
	rows := make([]measuredRow, 0)
	var rowsMu sync.Mutex

	appendRows := func(signalIndex int, fallbackSymbolOrder int, measured []*types.Measurement) {
		if len(measured) == 0 {
			return
		}

		rowsMu.Lock()
		defer rowsMu.Unlock()

		for rowIndex, measurement := range measured {
			if measurement == nil {
				continue
			}

			order, found := symbolOrder[measurement.Symbol]

			if !found {
				order = fallbackSymbolOrder
			}

			rows = append(rows, measuredRow{
				signalIndex: signalIndex,
				symbolOrder: order,
				rowOrder:    rowIndex,
				measurement: measurement,
			})
		}
	}

	localSignals := make([]activeMeasurementSignal, 0, len(active))

	for _, selected := range active {
		cohort, isCohort := selected.signal.(types.CohortSignal)

		if !isCohort {
			localSignals = append(localSignals, selected)
			continue
		}

		selected := selected

		group.Go(func() error {
			started := time.Now()
			measured := cohort.MeasureCohort(symbols, tick)

			if measurements.clocks != nil {
				measurements.clocks.observe(selected.signal.Name(), time.Since(started))
			}

			appendRows(selected.index, len(symbols), measured)
			return nil
		})
	}

	for symbolIndex, symbol := range symbols {
		symbolIndex := symbolIndex
		symbol := symbol

		group.Go(func() error {
			for _, selected := range localSignals {
				started := time.Now()
				measured := selected.signal.Measure(symbol, tick)

				if measurements.clocks != nil {
					measurements.clocks.observe(selected.signal.Name(), time.Since(started))
				}

				appendRows(selected.index, symbolIndex, measured)
			}

			return nil
		})
	}

	if waitErr := group.Wait(); waitErr != nil {
		return false, errnie.Error(errnie.Err(
			errnie.Internal,
			"measurements: failed to generate measurements",
			waitErr,
		))
	}

	sort.SliceStable(rows, func(leftIndex, rightIndex int) bool {
		left := rows[leftIndex]
		right := rows[rightIndex]

		if left.signalIndex != right.signalIndex {
			return left.signalIndex < right.signalIndex
		}

		if left.symbolOrder != right.symbolOrder {
			return left.symbolOrder < right.symbolOrder
		}

		if left.measurement.Symbol != right.measurement.Symbol {
			return left.measurement.Symbol < right.measurement.Symbol
		}

		if !left.measurement.At.Equal(right.measurement.At) {
			return left.measurement.At.Before(right.measurement.At)
		}

		if left.measurement.Peer != right.measurement.Peer {
			return left.measurement.Peer < right.measurement.Peer
		}

		return left.rowOrder < right.rowOrder
	})

	ready := false
	published := make([]*types.Measurement, 0, len(rows))

	for _, measured := range rows {
		measured.measurement.Tick = tick

		if thesis.Symbol(measured.measurement.Symbol).AppendMeasurement(
			measured.measurement.Source,
			measured.measurement,
		) {
			ready = true
		}

		published = append(published, measured.measurement)
	}

	completionFrame := datura.NewMap("activity", done)

	if len(published) > 0 {
		completionFrame["measurements"] = published
	}

	utils.Publish(measurements.ui, completionFrame)
	return ready, nil
}

func measurementSymbols(thesis *types.Thesis) ([]*types.Symbol, error) {
	names := make([]string, 0)
	var rangeErr error

	thesis.Symbols.Range(func(key, value any) bool {
		name, nameOK := key.(string)
		symbol, symbolOK := value.(*types.Symbol)

		if !nameOK || name == "" || !symbolOK || symbol == nil {
			rangeErr = fmt.Errorf("measurements: invalid thesis symbol entry")
			return false
		}

		names = append(names, name)
		return true
	})

	if rangeErr != nil {
		return nil, rangeErr
	}

	sort.Strings(names)
	symbols := make([]*types.Symbol, 0, len(names))

	for _, name := range names {
		raw, found := thesis.Symbols.Load(name)

		if !found {
			continue
		}

		symbol, ok := raw.(*types.Symbol)

		if !ok || symbol == nil {
			return nil, fmt.Errorf("measurements: invalid symbol state for %s", name)
		}

		symbols = append(symbols, symbol)
	}

	return symbols, nil
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
