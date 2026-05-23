package causal

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"sync/atomic"
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
	ctx      context.Context
	cancel   context.CancelFunc
	book     *kbook.Book
	trades   *trades.Trades
	ticker   *kticker.Ticker
	track    *TrackStore
	pairs    map[string]asset.Pair
	symbols  []string
	interval time.Duration
	queue    sync.Map
	seq      atomic.Int64
}

var _ engine.Signal = (*Causal)(nil)

/*
NewCausal wires live Kraken websocket observers into the engine signal.
*/
func NewCausal(
	ctx context.Context,
	observers []engine.Observer,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Causal, error) {
	ctx, cancel := context.WithCancel(ctx)

	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	book, tradesObserver, tickerObserver, err := resolveMarketObservers(observers)

	if err != nil {
		cancel()
		return nil, err
	}

	causal := &Causal{
		ctx:      ctx,
		cancel:   cancel,
		book:     book,
		trades:   tradesObserver,
		ticker:   tickerObserver,
		track:    NewTrackStore(),
		pairs:    pairs,
		symbols:  symbols,
		interval: interval,
	}

	return causal, errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
		"track":  causal.track,
		"pairs":  pairs,
	})
}

/*
Run advances the causal model on a fixed interval.
*/
func (causal *Causal) Run() {
	go func() {
		ticker := time.NewTicker(causal.interval)
		defer ticker.Stop()

		for {
			select {
			case <-causal.ctx.Done():
				return
			case tick := <-ticker.C:
				causal.scan(tick)
			}
		}
	}()
}

/*
Measure yields queued measurements for the trader.
*/
func (causal *Causal) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		causal.queue.Range(func(key, value any) bool {
			measurement, ok := value.(engine.Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement type: %T", value))
				causal.queue.Delete(key)
				return true
			}

			if !yield(measurement) {
				return false
			}

			causal.queue.Delete(key)
			return true
		})
	}
}

/*
Close stops rescoring.
*/
func (causal *Causal) Close() error {
	causal.cancel()
	return nil
}

func (causal *Causal) scan(now time.Time) {
	macro := causal.track.MacroMomentum(causal.symbols, func(symbol string) (float64, bool) {
		_, _, _, changePct, ok := causal.ticker.Quote(symbol)
		return changePct, ok
	})

	for _, symbol := range causal.symbols {
		confidence, reason, fired := causal.evaluate(symbol, macro, now)

		if !fired {
			continue
		}

		pair, ok := causal.pairs[symbol]

		if !ok {
			continue
		}

		causal.queue.Store(causal.seq.Add(1), engine.Measurement{
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

func (causal *Causal) evaluate(symbol string, macroMomentum float64, now time.Time) (float64, string, bool) {
	if !causal.track.PassesLiquidity(symbol) {
		return 0, "", false
	}

	price, priceOK := causal.ticker.Last(symbol)
	volumeBase, volumeOK := causal.ticker.VolumeBase(symbol)
	batchVolume, batchOK := causal.trades.BatchVolume(symbol)
	buyPressure, pressureOK := causal.trades.BuyPressure(symbol)
	spreadBPS, spreadOK := causal.book.SpreadBPS(symbol)
	imbalance, bookOK := causal.book.Imbalance(symbol)

	if !priceOK || !volumeOK || !batchOK || !pressureOK || !spreadOK || !bookOK {
		return 0, "", false
	}

	if price <= 0 || batchVolume <= 0 || spreadBPS <= 0 || imbalance <= 0 || buyPressure <= 0 {
		return 0, "", false
	}

	causal.track.ApplyTicker(symbol, price, volumeBase)

	localFlow := batchVolume * (buyPressure + 1) / 2
	liquidity := bookLiquidity(spreadBPS, batchVolume)

	sample, ready := causal.track.Record(symbol, macroMomentum, liquidity, localFlow, price, now)

	if !ready {
		return 0, "", false
	}

	return causal.track.Evaluate(symbol, sample)
}

func bookLiquidity(spreadBPS, batchVolume float64) float64 {
	if spreadBPS <= 0 || batchVolume <= 0 {
		return 0
	}

	return batchVolume / spreadBPS
}
