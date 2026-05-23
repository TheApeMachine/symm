package causal

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Causal applies Pearl's ladder: association, backdoor intervention, and counterfactual uplift.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, with Liquidity as backdoor control.
*/
type Causal struct {
	scanner *engine.Scanner
	book    *kbook.Book
	trades  *trades.Trades
	ticker  *kticker.Ticker
	track   *TrackStore
	pairs   map[string]asset.Pair
	symbols []string
}

var _ engine.Signal = (*Causal)(nil)

/*
NewCausal wires live Kraken websocket observers into the engine signal.
*/
func NewCausal(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Causal, error) {
	causal := &Causal{
		scanner: engine.NewScanner(ctx, interval),
		book:    book,
		trades:  tradesObserver,
		ticker:  tickerObserver,
		track:   NewTrackStore(),
		pairs:   pairs,
		symbols: symbols,
	}

	return causal, errnie.Require(map[string]any{
		"scanner": causal.scanner,
		"book":    book,
		"trades":  tradesObserver,
		"ticker":  tickerObserver,
		"track":   causal.track,
		"pairs":   pairs,
	})
}

/*
Run advances the causal model on a fixed interval.
*/
func (causal *Causal) Run() {
	causal.scanner.Run(causal.scan)
}

/*
Measure yields queued measurements for the trader.
*/
func (causal *Causal) Measure(ctx context.Context) iter.Seq[engine.Measurement] {
	return causal.scanner.Measure(ctx)
}

/*
Close stops rescoring.
*/
func (causal *Causal) Close() error {
	return causal.scanner.Close()
}

func (causal *Causal) scan(now time.Time) {
	macro := causal.track.MacroMomentum(causal.symbols, func(symbol string) (float64, bool) {
		_, _, _, changePct, ok := causal.ticker.Quote(symbol)
		return changePct, ok
	})

	for _, symbol := range causal.symbols {
		confidence, reason := causal.evaluate(symbol, macro, now)

		if confidence <= 0 {
			continue
		}

		pair, ok := causal.pairs[symbol]

		if !ok {
			continue
		}

		causal.scanner.Enqueue(engine.Measurement{
			Type:       engine.Causal,
			Source:     "causal",
			Regime:     "causal",
			Reason:     reason,
			Pairs:      []asset.Pair{pair},
			Confidence: confidence,
			Timeframe:  engine.Timeframe{Start: now.UnixNano(), End: now.UnixNano()},
		})
	}
}

func (causal *Causal) evaluate(symbol string, macroMomentum float64, now time.Time) (float64, string) {
	if !causal.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	price, priceOK := causal.ticker.Last(symbol)
	volumeBase, volumeOK := causal.ticker.VolumeBase(symbol)
	batchVolume, batchOK := causal.trades.BatchVolume(symbol)
	buyPressure, pressureOK := causal.trades.BuyPressure(symbol)
	spreadBPS, spreadOK := causal.book.SpreadBPS(symbol)
	imbalance, bookOK := causal.book.Imbalance(symbol)

	if !priceOK || !volumeOK || !batchOK || !pressureOK || !spreadOK || !bookOK {
		return 0, ""
	}

	if price <= 0 || batchVolume <= 0 || spreadBPS <= 0 || imbalance <= 0 || buyPressure <= 0 {
		return 0, ""
	}

	causal.track.ApplyTicker(symbol, price, volumeBase)

	localFlow := batchVolume * (buyPressure + 1) / 2
	liquidity := bookLiquidity(spreadBPS, batchVolume)

	sample, ready := causal.track.Record(symbol, macroMomentum, liquidity, localFlow, price, now)

	if !ready {
		return 0, ""
	}

	return causal.track.Evaluate(symbol, sample)
}

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
