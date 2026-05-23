package hawkes

import (
	"context"
	"iter"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	kbook "github.com/theapemachine/symm/kraken/book"
	"github.com/theapemachine/symm/kraken/market"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Hawkes detects self-exciting buy-side trade clustering via exponential kernel intensity.
*/
type Hawkes struct {
	scanner *engine.Scanner
	book    *kbook.Book
	trades  *trades.Trades
	ticker  *kticker.Ticker
	track   *TrackStore
	pairs   map[string]asset.Pair
	symbols []string
}

var _ engine.Signal = (*Hawkes)(nil)

/*
NewHawkes wires live Kraken websocket observers into the engine signal.
*/
func NewHawkes(
	ctx context.Context,
	book *kbook.Book,
	tradesObserver *trades.Trades,
	tickerObserver *kticker.Ticker,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Hawkes, error) {
	hawkes := &Hawkes{
		scanner: engine.NewScanner(ctx, interval),
		book:    book,
		trades:  tradesObserver,
		ticker:  tickerObserver,
		track:   NewTrackStore(),
		pairs:   pairs,
		symbols: symbols,
	}

	return hawkes, errnie.Require(map[string]any{
		"scanner": hawkes.scanner,
		"book":    book,
		"trades":  tradesObserver,
		"ticker":  tickerObserver,
		"track":   hawkes.track,
		"pairs":   pairs,
	})
}

/*
Run recalibrates Hawkes intensity on a fixed interval.
*/
func (hawkes *Hawkes) Run() {
	hawkes.scanner.Run(hawkes.scan)
}

/*
Measure yields queued measurements for the trader.
*/
func (hawkes *Hawkes) Measure(ctx context.Context) iter.Seq[engine.Measurement] {
	return hawkes.scanner.Measure(ctx)
}

/*
Close stops rescoring.
*/
func (hawkes *Hawkes) Close() error {
	return hawkes.scanner.Close()
}

func (hawkes *Hawkes) scan(now time.Time) {
	hawkes.ingest()

	for _, symbol := range hawkes.symbols {
		confidence := hawkes.evaluate(symbol, now)

		if confidence <= 0 {
			continue
		}

		pair, ok := hawkes.pairs[symbol]

		if !ok {
			continue
		}

		hawkes.scanner.Enqueue(engine.Measurement{
			Type:       engine.Momentum,
			Source:     "hawkes",
			Regime:     "momentum",
			Reason:     "cluster_buy",
			Pairs:      []asset.Pair{pair},
			Confidence: confidence,
			Timeframe:  engine.Timeframe{Start: now.UnixNano(), End: now.UnixNano()},
		})
	}
}

func (hawkes *Hawkes) ingest() {
	for _, symbol := range hawkes.symbols {
		last, lastOK := hawkes.ticker.Last(symbol)
		volumeBase, volumeOK := hawkes.ticker.VolumeBase(symbol)

		if lastOK && volumeOK {
			hawkes.track.ApplyTicker(symbol, last, volumeBase)
		}
	}
}

func (hawkes *Hawkes) evaluate(symbol string, now time.Time) float64 {
	if !hawkes.track.PassesLiquidity(symbol) {
		return 0
	}

	windowStart := now.Add(-hawkesTradeWindow)
	ticks, ok := hawkes.trades.RecentTicks(symbol, windowStart)

	if !ok {
		return 0
	}

	buyTimes, sellTimes := splitSideEvents(ticks, windowStart, now)
	buyFit := hawkes.track.FitSide(symbol, "buy", buyTimes, now)
	sellFit := hawkes.track.FitSide(symbol, "sell", sellTimes, now)

	if buyFit.mu <= 0 {
		return 0
	}

	asymmetry := buySellAsymmetry(buyFit, sellFit)
	confidence := excitationConfidence(buyFit, asymmetry)

	if confidence <= 0 {
		return 0
	}

	imbalance, bookOK := hawkes.book.Imbalance(symbol)

	if !bookOK || imbalance <= 0 {
		return 0
	}

	bookSide := imbalance

	if bookSide > 1 {
		bookSide = 1
	}

	score := confidence * bookSide

	return hawkes.track.RecordScore(symbol, score)
}

func splitSideEvents(
	ticks []market.TradeTick,
	windowStart, windowEnd time.Time,
) ([]time.Time, []time.Time) {
	buyTimes := make([]time.Time, 0, len(ticks))
	sellTimes := make([]time.Time, 0, len(ticks))

	for _, tick := range ticks {
		if tick.Timestamp.Before(windowStart) {
			continue
		}

		if tick.Timestamp.After(windowEnd) {
			continue
		}

		switch tick.Side {
		case "buy":
			buyTimes = append(buyTimes, tick.Timestamp)
		case "sell":
			sellTimes = append(sellTimes, tick.Timestamp)
		}
	}

	return buyTimes, sellTimes
}
