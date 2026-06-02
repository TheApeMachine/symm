package liquidity

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	minLiquidityPeers = 2
	rawSubscriberID   = "signal/liquidity:raw"
)

/*
Signal ranks a symbol's quote volume against the live cross-section of its peers
and maps the standing onto the scarcity perspective. It is a cross-asset signal:
the verdict for one symbol depends on where its quote volume sits in the peer
median. Confidence is classification clarity — margin to the nearest peer
quartile; SNR is how surprising that clarity is versus the symbol's own recent
baseline.

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
	tracked     sync.Map // symbol -> *perspectives.Category
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

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)

	activate.Boot("signal/liquidity ready")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(public.SocketMessage)

			if !ok {
				continue
			}

			switch envelope.Channel {
			case public.TickerChannel:
				for _, ticker := range errnie.Does(func() ([]market.TickerUpdate, error) {
					return market.DecodeTickers(&envelope)
				}).Or(func(err error) {
					errnie.Error(err)
				}).Value() {
					if ticker.Last <= 0 {
						continue
					}

					measurement, ok := signal.measure(ticker)

					if !ok {
						continue
					}

					measurement.SNR = signal.floor.Score(measurement.Symbol, measurement.Confidence)
					activate.Once("signal/liquidity:measurement")
					signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})
				}
			}
		}
	}
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
	category, evidence, err := liquidityReading(quoteVol, peers)

	if err != nil {
		return perspectives.Measurement{}, false
	}

	trackedRaw, _ := signal.tracked.LoadOrStore(
		row.Symbol,
		perspectives.NewCategory(perspectives.CategoryTypeNone),
	)
	tracked := trackedRaw.(*perspectives.Category)

	confidence, err := tracked.Observe(category, evidence)

	if err != nil {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Symbol:     row.Symbol,
		Source:     perspectives.SourceLiquidity,
		Category:   category,
		Last:       row.Last,
		Strength:   raw,
		Confidence: confidence,
	}, true
}

/*
strength is the raw distance of quote volume from the peer median, in either
direction, for dashboard gauges only.
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
