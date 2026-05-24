package engine

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
)

// TODO: REMOVE

/*
SymbolScanner selects symbols and reads ingest snapshots for one signal source.
*/
type SymbolScanner struct {
	Source  string
	Ingest  *Ingest
	Watch   *SymbolWatch
	Pairs   map[string]asset.Pair
	Symbols []string
}

/*
MeasureSymbols evaluates the active symbol set and yields non-zero measurements.
*/
func MeasureSymbols(
	ctx context.Context,
	scanner SymbolScanner,
	now time.Time,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
) iter.Seq[Measurement] {
	return func(yield func(Measurement) bool) {
		symbols := scanner.Symbols

		if scanner.Watch != nil {
			budget := config.System.MaxScanSymbols
			symbols = scanner.Watch.ScanSet(budget)
		}

		for _, symbol := range symbols {
			DrainTicks(ctx)

			if ctx.Err() != nil {
				return
			}

			snapshot := scanner.Ingest.Read(symbol)

			measurement, ok, err := evaluate(symbol, snapshot)

			if err != nil {
				return
			}

			if !ok {
				continue
			}

			pair, pairOK := scanner.Pairs[symbol]

			if !pairOK {
				continue
			}

			measurement.Source = scanner.Source
			measurement.Pairs = []asset.Pair{pair}

			if measurement.Timeframe.Start == 0 && measurement.Timeframe.End == 0 {
				measurement.Timeframe = Timeframe{Start: now.UnixNano(), End: now.UnixNano()}
			}

			if !yield(measurement) {
				return
			}
		}

		if scanner.Watch != nil {
			budget := config.System.MaxScanSymbols
			scanner.Watch.AdvanceRotation(budget)
			scanner.Watch.Decay(now)
		}
	}
}
