package pumpdump

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
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
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

var _ engine.Signal = (*PumpDump)(nil)

/*
NewPumpDump wires live Kraken websocket observers into the engine signal.
*/
func NewPumpDump(
	ctx context.Context,
	observers []engine.Observer,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*PumpDump, error) {
	ctx, cancel := context.WithCancel(ctx)

	if interval <= 0 {
		interval = 10 * time.Millisecond
	}

	book, tradesObserver, tickerObserver, err := resolveMarketObservers(observers)

	if err != nil {
		cancel()
		return nil, err
	}

	pumpdump := &PumpDump{
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

	return pumpdump, errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
		"track":  pumpdump.track,
		"pairs":  pairs,
	})
}

func resolveMarketObservers(
	observers []engine.Observer,
) (*kbook.Book, *trades.Trades, *kticker.Ticker, error) {
	var book *kbook.Book
	var tradesObserver *trades.Trades
	var tickerObserver *kticker.Ticker

	for _, observer := range observers {
		switch concrete := observer.(type) {
		case *kbook.Book:
			book = concrete
		case *trades.Trades:
			tradesObserver = concrete
		case *kticker.Ticker:
			tickerObserver = concrete
		}
	}

	return book, tradesObserver, tickerObserver, errnie.Require(map[string]any{
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
	})
}

/*
Run samples microstructure on a fixed interval.
*/
func (pumpdump *PumpDump) Run() {
	go func() {
		ticker := time.NewTicker(pumpdump.interval)
		defer ticker.Stop()

		for {
			select {
			case <-pumpdump.ctx.Done():
				return
			case tick := <-ticker.C:
				pumpdump.scan(tick)
			}
		}
	}()
}

/*
Measure yields queued measurements for the trader.
*/
func (pumpdump *PumpDump) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		pumpdump.queue.Range(func(key, value any) bool {
			measurement, ok := value.(engine.Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement type: %T", value))
				pumpdump.queue.Delete(key)
				return true
			}

			if !yield(measurement) {
				return false
			}

			pumpdump.queue.Delete(key)
			return true
		})
	}
}

/*
Close stops rescoring.
*/
func (pumpdump *PumpDump) Close() error {
	pumpdump.cancel()
	return nil
}

func (pumpdump *PumpDump) scan(now time.Time) {
	pumpdump.ingest(now)
	pumpdump.track.RollBuckets(now)

	for _, symbol := range pumpdump.symbols {
		confidence, fired := pumpdump.evaluate(symbol)

		if !fired {
			continue
		}

		pair, ok := pumpdump.pairs[symbol]

		if !ok {
			continue
		}

		pumpdump.queue.Store(pumpdump.seq.Add(1), engine.Measurement{
			Type:       engine.Pump,
			Source:     "pumpdump",
			Regime:     "pump",
			Reason:     "actual_pump",
			Pairs:      []asset.Pair{pair},
			Confidence: confidence,
			Timeframe:  engine.Timeframe{Start: now.UnixNano(), End: now.UnixNano()},
		})
	}
}

func (pumpdump *PumpDump) ingest(now time.Time) {
	for _, symbol := range pumpdump.symbols {
		last, lastOK := pumpdump.ticker.Last(symbol)
		volumeBase, volumeOK := pumpdump.ticker.VolumeBase(symbol)

		if lastOK && volumeOK {
			pumpdump.track.ApplyTicker(symbol, last, volumeBase)
		}

		batchVolume, batchOK := pumpdump.trades.BatchVolume(symbol)

		if batchOK {
			pumpdump.track.AddVolume(symbol, batchVolume)
		}

		spreadBPS, spreadOK := pumpdump.book.SpreadBPS(symbol)

		if spreadOK {
			pumpdump.track.RecordSpread(symbol, spreadBPS)
		}

		_ = now
	}
}

func (pumpdump *PumpDump) evaluate(symbol string) (float64, bool) {
	if !pumpdump.track.PassesLiquidity(symbol) {
		return 0, false
	}

	volumeRatio, volumeSpike := pumpdump.track.VolumeSpike(symbol)

	if !volumeSpike {
		return 0, false
	}

	if !pumpdump.track.PriceFlat(symbol) {
		return 0, false
	}

	spreadBPS, spreadOK := pumpdump.book.SpreadBPS(symbol)

	if !spreadOK || !pumpdump.track.SpreadTight(symbol, spreadBPS) {
		return 0, false
	}

	imbalance, bookOK := pumpdump.book.Imbalance(symbol)
	buyPressure, tradeOK := pumpdump.trades.BuyPressure(symbol)

	if !bookOK || !tradeOK {
		return 0, false
	}

	micro := precursorScore(imbalance, buyPressure)

	if micro <= 0 {
		return 0, false
	}

	return volumeRatio * micro, true
}

/*
precursorScore requires bid-side book pressure confirmed by executed market buys.
*/
func precursorScore(imbalance, buyPressure float64) float64 {
	if imbalance <= 0 || buyPressure <= 0 {
		return 0
	}

	bookSide := imbalance

	if bookSide > 1 {
		bookSide = 1
	}

	buySide := (buyPressure + 1) / 2

	return bookSide * buySide
}
