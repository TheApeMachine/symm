package pumpdump

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
PumpDump detects pre-pump microstructure from Kraken book, trade, and ticker streams.
*/
type PumpDump struct {
	scanner *engine.Scanner
	book    *kbook.Book
	trades  *trades.Trades
	ticker  *kticker.Ticker
	track   *TrackStore
	pairs   map[string]asset.Pair
	symbols []string
}

var _ engine.Signal = (*PumpDump)(nil)

/*
NewPumpDump wires live Kraken websocket observers into the engine signal.
*/
func NewPumpDump(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*PumpDump, error) {
	pumpdump := &PumpDump{
		scanner: engine.NewScanner(ctx, interval),
		book:    book,
		trades:  tradesObserver,
		ticker:  tickerObserver,
		track:   NewTrackStore(),
		pairs:   pairs,
		symbols: symbols,
	}

	return pumpdump, errnie.Require(map[string]any{
		"scanner": pumpdump.scanner,
		"book":    book,
		"trades":  tradesObserver,
		"ticker":  tickerObserver,
		"track":   pumpdump.track,
		"pairs":   pairs,
	})
}

/*
Run samples microstructure on a fixed interval.
*/
func (pumpdump *PumpDump) Run() {
	pumpdump.scanner.Run(pumpdump.scan)
}

/*
Measure yields queued measurements for the trader.
*/
func (pumpdump *PumpDump) Measure(ctx context.Context) iter.Seq[engine.Measurement] {
	return pumpdump.scanner.Measure(ctx)
}

/*
Close stops rescoring.
*/
func (pumpdump *PumpDump) Close() error {
	return pumpdump.scanner.Close()
}

func (pumpdump *PumpDump) scan(now time.Time) {
	pumpdump.ingest(now)
	pumpdump.track.RollBuckets(now)

	for _, symbol := range pumpdump.symbols {
		confidence, reason := pumpdump.evaluate(symbol)

		if confidence <= 0 {
			continue
		}

		pair, ok := pumpdump.pairs[symbol]

		if !ok {
			continue
		}

		if reason == "" {
			reason = "precursor"
		}

		pumpdump.scanner.Enqueue(engine.Measurement{
			Type:       engine.Pump,
			Source:     "pumpdump",
			Regime:     "pump",
			Reason:     reason,
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

func (pumpdump *PumpDump) evaluate(symbol string) (float64, string) {
	if !pumpdump.track.PassesLiquidity(symbol) {
		return 0, ""
	}

	volumeRatio, volumeSpike := pumpdump.track.VolumeSpike(symbol)
	imbalance, bookOK := pumpdump.book.Imbalance(symbol)
	buyPressure, tradeOK := pumpdump.trades.BuyPressure(symbol)

	if !bookOK || !tradeOK {
		return 0, ""
	}

	micro := precursorScore(imbalance, buyPressure)

	if micro <= 0 || volumeRatio <= 0 {
		return 0, ""
	}

	confidence := volumeRatio * micro
	reason := "precursor"

	if !volumeSpike {
		return confidence, reason
	}

	if !pumpdump.track.PriceFlat(symbol) {
		return confidence, reason
	}

	spreadBPS, spreadOK := pumpdump.book.SpreadBPS(symbol)

	if !spreadOK || !pumpdump.track.SpreadTight(symbol, spreadBPS) {
		return confidence, reason
	}

	return confidence, "actual_pump"
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
