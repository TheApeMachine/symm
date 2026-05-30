package sentiment

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const minBreadth = 0.55

/*
Signal measures cross-section bullish breadth from ticker change percentages and
maps it onto the sentiment perspective. It is cross-asset: the verdict for one
symbol depends on how much of the universe is green, so SNR is the decisiveness
of that breadth (its odds away from a 50/50 split).

| Category        | Cross-section                                  |
|:----------------|:-----------------------------------------------|
| Risk-On Surge   | majority of the universe rising (>= 55%)       |
| Divergent Move  | this symbol leads while breadth is weak        |
| Systemic Slump  | breadth weak and this symbol is not a leader   |
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map // symbol -> float64 (change percent)
	floor       *adaptive.SNRField
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		floor:       adaptive.NewSNRField(),
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup(
		"measurements", 10*time.Millisecond,
	)

	return signal
}

func (signal *Signal) Tick() error {
	for row := range market.NewTickerSubscription(signal.ctx, config.System.Symbols...) {
		if row == nil || row.Last <= 0 || row.ChangePct == 0 {
			continue
		}

		signal.symbols.Store(row.Symbol, row.ChangePct)

		measurement, ok := signal.measure(row.ChangePct)

		if !ok {
			continue
		}

		measurement.Symbol = row.Symbol
		measurement.Last = row.Last
		measurement.SNR = signal.floor.Score(row.Symbol, measurement.SNR)
		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	return signal.ctx.Err()
}

// measure classifies one symbol against the live cross-section breadth.
func (signal *Signal) measure(change float64) (perspectives.Measurement, bool) {
	breadth, topChange, ok := signal.breadth()

	if !ok {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceSentiment,
		Category: signal.category(breadth, change, topChange),
		SNR:      signal.snr(breadth),
	}, true
}

// breadth returns the fraction of the universe that is rising and the strongest
// positive change observed.
func (signal *Signal) breadth() (fraction, topChange float64, ok bool) {
	positive := 0
	total := 0

	signal.symbols.Range(func(_, value any) bool {
		change := value.(float64)

		if change == 0 {
			return true
		}

		total++

		if change > topChange {
			topChange = change
		}

		if change > 0 {
			positive++
		}

		return true
	})

	if total == 0 {
		return 0, 0, false
	}

	return float64(positive) / float64(total), topChange, true
}

// category maps breadth and this symbol's leadership onto the sentiment perspective.
func (signal *Signal) category(breadth, change, topChange float64) perspectives.CategoryType {
	if breadth >= minBreadth {
		return perspectives.CategoryRiskOnSurge
	}

	leaderShare := 0.0

	if topChange > 0 {
		leaderShare = math.Abs(change) / topChange
	}

	if leaderShare >= 0.5 && change != 0 {
		return perspectives.CategoryDivergentMove
	}

	return perspectives.CategorySystemicSlump
}

// snr is the decisiveness of the breadth split — its odds away from 50/50.
func (signal *Signal) snr(breadth float64) float64 {
	if breadth <= 0 || breadth >= 1 {
		return 1
	}

	if breadth >= 0.5 {
		return breadth / (1 - breadth)
	}

	return (1 - breadth) / breadth
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
