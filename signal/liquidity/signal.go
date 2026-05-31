package liquidity

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const minLiquidityPeers = 2

/*
Signal ranks a symbol's quote volume against the live cross-section of its peers
and maps the standing onto the scarcity perspective. It is a cross-asset signal:
the verdict for one symbol depends on where its quote volume sits in the peer
median, so SNR is the dimensionless distance from that median (either side).

| Category          | Quote Volume vs peer median | Market "Feel"     |
|:------------------|:----------------------------|:------------------|
| Robust Liquidity  | well above (>= 1.25x)       | Deep / easy fills |
| Median Depth      | around the median           | Normal            |
| Extreme Scarcity  | well below (< 0.75x)        | Thin / fragile    |
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	symbols     sync.Map // symbol -> float64 (daily quote volume)
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
		if row == nil || row.Last <= 0 {
			continue
		}

		measurement, ok := signal.measure(*row)

		if !ok {
			continue
		}

		signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
	}

	return signal.ctx.Err()
}

/*
measure records the latest quote volume for the ticking symbol and ranks it
against the live peer cross-section.
*/
func (signal *Signal) measure(row market.TickerUpdate) (perspectives.Measurement, bool) {
	signal.symbols.Store(row.Symbol, row.Volume*row.Last)

	quoteVol, peers := signal.crossSection(row.Symbol)

	if quoteVol <= 0 || len(peers) < minLiquidityPeers {
		return perspectives.Measurement{}, false
	}

	median := numeric.PercentileSorted(numeric.CopySorted(peers), 0.5)

	if median <= 0 {
		return perspectives.Measurement{}, false
	}

	ratio := quoteVol / median
	raw := signal.strength(ratio)

	return perspectives.FinalizeMeasurement(perspectives.Measurement{
		Symbol:   row.Symbol,
		Source:   perspectives.SourceLiquidity,
		Category: signal.category(quoteVol, peers),
		Last:     row.Last,
	}, raw, "scarcity"), true
}

/*
category maps quote volume relative to the live peer cross-section onto the
scarcity perspective using quartiles of peer quote volume.
*/
func (signal *Signal) category(quoteVol float64, peers []float64) perspectives.CategoryType {
	sorted := numeric.CopySorted(peers)
	q1 := numeric.PercentileSorted(sorted, 0.25)
	q3 := numeric.PercentileSorted(sorted, 0.75)

	switch {
	case quoteVol >= q3:
		return perspectives.CategoryRobustLiquidity
	case quoteVol >= q1:
		return perspectives.CategoryMedianDepth
	default:
		return perspectives.CategoryExtremeScarcity
	}
}

/*
strength is the raw distance of quote volume from the peer median, in either
direction; it is scored against the symbol's own noise floor to form the SNR.
*/
func (signal *Signal) strength(ratio float64) float64 {
	if ratio < 1 {
		return 1 / ratio
	}

	return ratio
}

/*
crossSection returns the symbol's own quote volume and the peer volumes.
*/
func (signal *Signal) crossSection(symbol string) (own float64, peers []float64) {
	peers = make([]float64, 0)

	signal.symbols.Range(func(key, value any) bool {
		volume := value.(float64)

		if volume <= 0 {
			return true
		}

		if key.(string) == symbol {
			own = volume
			return true
		}

		peers = append(peers, volume)

		return true
	})

	return own, peers
}

func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
