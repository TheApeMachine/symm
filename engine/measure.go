package engine

import (
	"context"
	"fmt"
	"iter"
	"runtime"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
SymbolScanner selects symbols and reads ingest snapshots for one signal source.
*/
type SymbolScanner struct {
	Source  string
	Market  MarketReader
	Watch   *SymbolWatch
	Pairs   map[string]asset.Pair
	Symbols []string
	Pool    *qpool.Q
}

type symbolEvalResult struct {
	measurement Measurement
	ok          bool
	err         error
}

/*
MeasureSymbols evaluates the active symbol set and yields non-zero measurements.
When Pool is set, independent symbol evaluations run on qpool workers while the
caller drains orchestrator tickables between completions.
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

		if len(symbols) == 0 {
			finishSymbolScan(ctx, scanner, now)

			return
		}

		if scanner.Pool != nil && len(symbols) > 1 {
			if !measureSymbolsParallel(ctx, scanner, now, symbols, evaluate, yield) {
				return
			}

			finishSymbolScan(ctx, scanner, now)

			return
		}

		if !measureSymbolsSerial(ctx, scanner, now, symbols, evaluate, yield) {
			return
		}

		finishSymbolScan(ctx, scanner, now)
	}
}

func finishSymbolScan(ctx context.Context, scanner SymbolScanner, now time.Time) {
	if scanner.Watch == nil {
		return
	}

	if ctx.Err() != nil {
		return
	}

	budget := config.System.MaxScanSymbols
	scanner.Watch.AdvanceRotation(budget)
	scanner.Watch.Decay(now)
}

func measureSymbolsSerial(
	ctx context.Context,
	scanner SymbolScanner,
	now time.Time,
	symbols []string,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
	yield func(Measurement) bool,
) bool {
	for _, symbol := range symbols {
		DrainTicks(ctx)

		if ctx.Err() != nil {
			return false
		}

		result := evaluateSymbol(scanner, now, symbol, evaluate)

		if result.err != nil {
			return false
		}

		if !result.ok {
			continue
		}

		if !yield(result.measurement) {
			return false
		}
	}

	return true
}

func measureSymbolsParallel(
	ctx context.Context,
	scanner SymbolScanner,
	now time.Time,
	symbols []string,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
	yield func(Measurement) bool,
) bool {
	pending := make([]chan *qpool.QValue[any], len(symbols))

	for index, symbol := range symbols {
		symbolIndex := index
		symbolName := symbol

		pending[symbolIndex] = scanner.Pool.ScheduleFast(ctx, func(jobCtx context.Context) (any, error) {
			if jobCtx.Err() != nil {
				return symbolEvalResult{err: jobCtx.Err()}, nil
			}

			return evaluateSymbol(scanner, now, symbolName, evaluate), nil
		})
	}

	remaining := len(pending)

	for remaining > 0 {
		DrainTicks(ctx)

		if ctx.Err() != nil {
			return false
		}

		progress := false

		for index, resultChannel := range pending {
			if resultChannel == nil {
				continue
			}

			select {
			case value := <-resultChannel:
				pending[index] = nil
				remaining--
				progress = true

				if value == nil {
					return false
				}

				if value.Error != nil {
					return false
				}

				result, ok := value.Value.(symbolEvalResult)

				if !ok {
					return false
				}

				if result.err != nil {
					return false
				}

				if !result.ok {
					continue
				}

				if !yield(result.measurement) {
					return false
				}
			default:
			}
		}

		if !progress {
			runtime.Gosched()
		}
	}

	return true
}

func evaluateSymbol(
	scanner SymbolScanner,
	now time.Time,
	symbol string,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
) symbolEvalResult {
	var snapshot Snapshot

	if scanner.Market != nil {
		snapshot = scanner.Market.ReadFresh(
			symbol,
			now,
			config.System.SnapshotFreshnessTTL,
		)
	}

	measurement, ok, err := evaluate(symbol, snapshot)

	if err != nil {
		return symbolEvalResult{err: err}
	}

	if !ok {
		return symbolEvalResult{}
	}

	pair, pairOK := scanner.Pairs[symbol]

	if !pairOK {
		return symbolEvalResult{}
	}

	measurement.Source = scanner.Source
	measurement.Pairs = []asset.Pair{pair}

	if measurement.Timeframe.Start == 0 && measurement.Timeframe.End == 0 {
		measurement.Timeframe = Timeframe{Start: now.UnixNano(), End: now.UnixNano()}
	}

	return symbolEvalResult{
		measurement: measurement,
		ok:          true,
	}
}

/*
RunSymbolJobs schedules independent per-symbol jobs on qpool and waits for all
completions while draining orchestrator tickables between receives.
*/
func RunSymbolJobs(
	ctx context.Context,
	pool *qpool.Q,
	symbols []string,
	job func(symbol string) error,
) error {
	if pool == nil || len(symbols) == 0 {
		for _, symbol := range symbols {
			DrainTicks(ctx)

			if ctx.Err() != nil {
				return ctx.Err()
			}

			if err := job(symbol); err != nil {
				return err
			}
		}

		return nil
	}

	if len(symbols) == 1 {
		DrainTicks(ctx)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		return job(symbols[0])
	}

	pending := make([]chan *qpool.QValue[any], len(symbols))

	for index, symbol := range symbols {
		symbolName := symbol

		pending[index] = pool.ScheduleFast(ctx, func(jobCtx context.Context) (any, error) {
			if jobCtx.Err() != nil {
				return nil, jobCtx.Err()
			}

			return nil, job(symbolName)
		})
	}

	remaining := len(pending)

	for remaining > 0 {
		DrainTicks(ctx)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		progress := false

		for index, resultChannel := range pending {
			if resultChannel == nil {
				continue
			}

			select {
			case value := <-resultChannel:
				pending[index] = nil
				remaining--
				progress = true

				if value == nil {
					return fmt.Errorf("engine: symbol job returned nil")
				}

				if value.Error != nil {
					return value.Error
				}
			default:
			}
		}

		if !progress {
			runtime.Gosched()
		}
	}

	return nil
}
