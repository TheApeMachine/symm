package hindsight

import (
	"context"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/store"
)

/*
stitch is one symbol's print on a stored workspace tick. The frame is shared
by every symbol on that tick, so the run is held once.
*/
type stitch struct {
	frame *data.Value
	tick  *broker.SymbolTick
}

/*
runTape is one run, split into a continuous tape per symbol, in stored order.
*/
type runTape struct {
	all      []stitch
	bySymbol map[string][]stitch
}

/*
tape returns the run's stitched tapes, loading them from storage the first
time. Later scans and symbol streams use this copy.
*/
func (service *Service) tape(ctx context.Context, run uint64) (*runTape, error) {
	return service.loadTape(ctx, run, func(ctx context.Context) (*runTape, error) {
		return LoadRun(ctx, service.client, run, nil)
	})
}

/*
loadTape single-flights the download of one run. A caller whose context is
canceled does not publish that cancellation: the next waiter loads the run
itself. Publishing it made a replaced index request fail the one that
replaced it, and the page showed "context canceled".
*/
func (service *Service) loadTape(
	ctx context.Context,
	run uint64,
	load func(context.Context) (*runTape, error),
) (*runTape, error) {
	for {
		service.tapesMu.Lock()

		if loaded := service.tapes[run]; loaded != nil {
			service.tapesMu.Unlock()
			return loaded, nil
		}

		if wait := service.tapeWait[run]; wait != nil {
			service.tapesMu.Unlock()

			select {
			case <-wait:
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			service.tapesMu.Lock()
			loaded, err := service.tapes[run], service.tapeErr[run]
			service.tapesMu.Unlock()

			if loaded != nil || err != nil {
				return loaded, err
			}

			// The load we waited on belonged to a canceled caller.
			continue
		}

		wait := make(chan struct{})
		service.tapeWait[run] = wait
		service.tapesMu.Unlock()

		loaded, err := load(ctx)

		service.tapesMu.Lock()

		if err == nil {
			service.tapes[run] = loaded
			service.tapeErr[run] = nil
		} else if ctx.Err() == nil {
			service.tapeErr[run] = err
		} else {
			service.tapeErr[run] = nil
		}

		close(wait)
		delete(service.tapeWait, run)
		service.tapesMu.Unlock()

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return loaded, err
	}
}

/*
LoadRun downloads run once and stitches each symbol's prints into one tape.
onDownload runs after each object read.
*/
func LoadRun(ctx context.Context, client *store.Client, run uint64, onDownload func()) (*runTape, error) {
	keys := client.ListKeys(data.TickPrefix(run))
	loaded, err := assemble(ctx, client, keys, onDownload, nil)

	if err == nil && client != nil {
		_ = WriteRunManifest(client, run, keys)
	}

	return loaded, err
}

/*
Assemble stitches an already listed key set. detect uses it so the progress
bar can be sized before the downloads start.
*/
func Assemble(ctx context.Context, client *store.Client, keys []string, onDownload func()) (*runTape, error) {
	return assemble(ctx, client, keys, onDownload, nil)
}

func assemble(
	ctx context.Context,
	client *store.Client,
	keys []string,
	onDownload func(),
	onRow func(*data.Value, *broker.SymbolTick) error,
) (*runTape, error) {
	tape := &runTape{bySymbol: map[string][]stitch{}}

	for frame, err := range store.ParallelValuesProgress(client, keys, onDownload) {
		if err != nil {
			return nil, err
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ticks, err := broker.ReadTick(frame)

		if err != nil {
			return nil, err
		}

		for _, tick := range ticks {
			if tick == nil || tick.Symbol == "" {
				continue
			}

			row := stitch{frame: frame, tick: tick}
			tape.all = append(tape.all, row)
			tape.bySymbol[tick.Symbol] = append(tape.bySymbol[tick.Symbol], row)

			if onRow != nil {
				if err := onRow(frame, tick); err != nil {
					return nil, err
				}
			}
		}
	}

	if len(tape.all) == 0 {
		return nil, errnie.Err(errnie.NotFound, "[hindsight] run has no stored ticks", nil)
	}

	return tape, nil
}

func (tape *runTape) prices(symbol string, from, to uint64) []float64 {
	prices := make([]float64, 0)

	for _, row := range tape.bySymbol[symbol] {
		if row.frame.Tick < from || row.frame.Tick > to {
			continue
		}

		for _, trade := range row.tick.Trades {
			price := trade.Price.Float64()

			if price > 0 {
				prices = append(prices, price)
			}
		}
	}

	return prices
}

/*
scanSymbols runs one detector over each symbol tape. The detectors do not
share state, so the scans run together.
*/
/*
ScanRun runs one detector over each symbol tape in parallel.
*/
func ScanRun(tape *runTape, retrace float64, want map[string]struct{}) []*data.Value {
	if tape == nil {
		return nil
	}

	return scanSymbols(tape, retrace, want)
}

func scanSymbol(tape *runTape, symbol string, retrace float64) []*data.Value {
	if tape == nil {
		return nil
	}

	rows := tape.bySymbol[symbol]

	if len(rows) == 0 {
		return nil
	}

	detector := scanningDetector()
	detector.SetRetrace(retrace)
	out := make([]*data.Value, 0)

	for _, row := range rows {
		out = append(out, detector.Replay(row.frame, row.tick)...)
	}

	return append(out, detector.Finish()...)
}

func scanSymbols(tape *runTape, retrace float64, want map[string]struct{}) []*data.Value {
	symbols := make([]string, 0, len(tape.bySymbol))

	for symbol := range tape.bySymbol {
		if len(want) > 0 {
			if _, ok := want[symbol]; !ok {
				continue
			}
		}

		symbols = append(symbols, symbol)
	}

	drafts := make([][]*data.Value, len(symbols))
	var wg sync.WaitGroup

	for i, symbol := range symbols {
		wg.Add(1)

		go func(i int, symbol string) {
			defer wg.Done()
			drafts[i] = scanSymbol(tape, symbol, retrace)
		}(i, symbol)
	}

	wg.Wait()

	flat := make([]*data.Value, 0)

	for _, batch := range drafts {
		flat = append(flat, batch...)
	}

	return flat
}
