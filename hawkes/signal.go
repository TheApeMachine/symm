package hawkes

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
	"github.com/theapemachine/symm/kraken/market"
	kticker "github.com/theapemachine/symm/kraken/ticker"
	"github.com/theapemachine/symm/kraken/trades"
)

/*
Hawkes detects self-exciting buy-side trade clustering via exponential kernel intensity.
*/
type Hawkes struct {
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

var _ engine.Signal = (*Hawkes)(nil)

/*
NewHawkes wires live Kraken websocket observers into the engine signal.
*/
func NewHawkes(
	ctx context.Context,
	observers []engine.Observer,
	pairs map[string]asset.Pair,
	symbols []string,
	interval time.Duration,
) (*Hawkes, error) {
	ctx, cancel := context.WithCancel(ctx)

	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	book, tradesObserver, tickerObserver, err := resolveMarketObservers(observers)

	if err != nil {
		cancel()
		return nil, err
	}

	hawkes := &Hawkes{
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

	return hawkes, errnie.Require(map[string]any{
		"ctx":    ctx,
		"cancel": cancel,
		"book":   book,
		"trades": tradesObserver,
		"ticker": tickerObserver,
		"track":  hawkes.track,
		"pairs":  pairs,
	})
}

/*
Run recalibrates Hawkes intensity on a fixed interval.
*/
func (hawkes *Hawkes) Run() {
	go func() {
		ticker := time.NewTicker(hawkes.interval)
		defer ticker.Stop()

		for {
			select {
			case <-hawkes.ctx.Done():
				return
			case tick := <-ticker.C:
				hawkes.scan(tick)
			}
		}
	}()
}

/*
Measure yields queued measurements for the trader.
*/
func (hawkes *Hawkes) Measure(_ context.Context) iter.Seq[engine.Measurement] {
	return func(yield func(engine.Measurement) bool) {
		hawkes.queue.Range(func(key, value any) bool {
			measurement, ok := value.(engine.Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement type: %T", value))
				hawkes.queue.Delete(key)
				return true
			}

			if !yield(measurement) {
				return false
			}

			hawkes.queue.Delete(key)
			return true
		})
	}
}

/*
Close stops rescoring.
*/
func (hawkes *Hawkes) Close() error {
	hawkes.cancel()
	return nil
}

func (hawkes *Hawkes) scan(now time.Time) {
	hawkes.ingest()

	for _, symbol := range hawkes.symbols {
		confidence, fired := hawkes.evaluate(symbol, now)

		if !fired {
			continue
		}

		pair, ok := hawkes.pairs[symbol]

		if !ok {
			continue
		}

		hawkes.queue.Store(hawkes.seq.Add(1), engine.Measurement{
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

func (hawkes *Hawkes) evaluate(symbol string, now time.Time) (float64, bool) {
	if !hawkes.track.PassesLiquidity(symbol) {
		return 0, false
	}

	ticks, ok := hawkes.trades.RecentTicks(symbol, time.Time{})

	if !ok {
		return 0, false
	}

	buyTimes, sellTimes := splitSideEvents(ticks, now)
	buyFit := fitSide(buyTimes, now)
	sellFit := fitSide(sellTimes, now)

	if buyFit.mu <= 0 {
		return 0, false
	}

	asymmetry := buySellAsymmetry(buyFit, sellFit)
	confidence := excitationConfidence(buyFit, asymmetry)

	if confidence <= 0 {
		return 0, false
	}

	imbalance, bookOK := hawkes.book.Imbalance(symbol)

	if !bookOK || imbalance <= 0 {
		return 0, false
	}

	bookSide := imbalance

	if bookSide > 1 {
		bookSide = 1
	}

	score := confidence * bookSide

	return hawkes.track.ConfidenceSpike(symbol, score)
}

func splitSideEvents(ticks []market.TradeTick, horizon time.Time) ([]time.Time, []time.Time) {
	buyTimes := make([]time.Time, 0, len(ticks))
	sellTimes := make([]time.Time, 0, len(ticks))

	for _, tick := range ticks {
		if tick.Timestamp.After(horizon) {
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
