package engine

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
SignalBase holds shared signal wiring: scanner queue, ingest facade, and symbol watch.
*/
type SignalBase struct {
	source  string
	scanner *Scanner
	ingest  *Ingest
	watch   *SymbolWatch
	pairs   map[string]asset.Pair
	symbols []string
}

/*
NewSignalBase wires observers and the shared symbol watch into one signal skeleton.
*/
func NewSignalBase(
	ctx context.Context,
	source string,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	watch *SymbolWatch,
) (*SignalBase, error) {
	base := &SignalBase{
		source:  source,
		scanner: NewScanner(ctx),
		ingest:  NewIngest(book, tradesObserver, tickerObserver),
		watch:   watch,
		pairs:   pairs,
		symbols: append([]string(nil), symbols...),
	}

	return base, errnie.Require(map[string]any{
		"source":  source,
		"scanner": base.scanner,
		"ingest":  base.ingest,
		"watch":   watch,
		"pairs":   pairs,
	})
}

/*
Source identifies this signal in queue telemetry.
*/
func (signalBase *SignalBase) Source() string {
	return signalBase.source
}

/*
Stats returns queue counters for telemetry.
*/
func (signalBase *SignalBase) Stats() QueueStats {
	return signalBase.scanner.Stats()
}

/*
Measure yields queued measurements for the trader.
*/
func (signalBase *SignalBase) Measure(ctx context.Context) iter.Seq[Measurement] {
	return signalBase.scanner.Measure(ctx)
}

/*
Close stops the passive scanner queue.
*/
func (signalBase *SignalBase) Close() error {
	return signalBase.scanner.Close()
}

/*
Ingest exposes the shared observer read facade.
*/
func (signalBase *SignalBase) Ingest() *Ingest {
	return signalBase.ingest
}

/*
Symbols returns the full subscribed universe.
*/
func (signalBase *SignalBase) Symbols() []string {
	return append([]string(nil), signalBase.symbols...)
}

/*
Pair resolves one subscribed asset pair.
*/
func (signalBase *SignalBase) Pair(symbol string) (asset.Pair, bool) {
	pair, ok := signalBase.pairs[symbol]

	return pair, ok
}

/*
ScanSymbols evaluates a prioritized subset and enqueues non-zero measurements.
*/
func (signalBase *SignalBase) ScanSymbols(
	now time.Time,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
) error {
	if signalBase.watch == nil {
		return signalBase.scanAll(now, evaluate)
	}

	budget := config.System.MaxScanSymbols
	symbols := signalBase.watch.ScanSet(budget)

	for _, symbol := range symbols {
		snapshot := signalBase.ingest.Read(symbol)

		measurement, ok, err := evaluate(symbol, snapshot)

		if err != nil {
			return err
		}

		if !ok {
			continue
		}

		pair, pairOK := signalBase.Pair(symbol)

		if !pairOK {
			continue
		}

		measurement.Source = signalBase.source
		measurement.Pairs = []asset.Pair{pair}

		if measurement.Timeframe.Start == 0 && measurement.Timeframe.End == 0 {
			measurement.Timeframe = Timeframe{Start: now.UnixNano(), End: now.UnixNano()}
		}

		if err := signalBase.scanner.Enqueue(measurement); err != nil {
			return err
		}
	}

	signalBase.watch.AdvanceRotation(budget)
	signalBase.watch.Decay(now)

	return nil
}

func (signalBase *SignalBase) scanAll(
	now time.Time,
	evaluate func(symbol string, snapshot Snapshot) (Measurement, bool, error),
) error {
	for _, symbol := range signalBase.symbols {
		snapshot := signalBase.ingest.Read(symbol)

		measurement, ok, err := evaluate(symbol, snapshot)

		if err != nil {
			return err
		}

		if !ok {
			continue
		}

		pair, pairOK := signalBase.Pair(symbol)

		if !pairOK {
			continue
		}

		measurement.Source = signalBase.source
		measurement.Pairs = []asset.Pair{pair}
		measurement.Timeframe = Timeframe{Start: now.UnixNano(), End: now.UnixNano()}

		if err := signalBase.scanner.Enqueue(measurement); err != nil {
			return err
		}
	}

	return nil
}
