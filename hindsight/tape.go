package hindsight

import (
	"context"
	"fmt"
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
LoadRun downloads run once and stitches each symbol's prints into one tape.
onDownload runs after each object read.
*/
func LoadRun(ctx context.Context, client *store.Client, run uint64, onDownload func()) (*runTape, error) {
	var keys []string

	prefix := "workspace/tick/aggregate"

	if run > 0 {
		prefix = fmt.Sprintf("%s/%019d", prefix, run)
	}

	for key := range client.ListKeys(prefix) {
		keys = append(keys, key)
	}

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

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		frame, err := readObject(ctx, client, key)

		if err != nil {
			return nil, err
		}

		if onDownload != nil {
			onDownload()
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
