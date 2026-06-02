package liquidity

import (
	"context"
	"errors"
	"fmt"
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
quartile; SNR scores category standout — peer deviation from the median — against
the symbol's own recent baseline.

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
				tickers, err := market.DecodeTickers(&envelope)

				if err != nil {
					errnie.Error(err, "liquidity: decode tickers")
					continue
				}

				if err := signal.publishTickers(tickers); err != nil {
					errnie.Error(err, "liquidity: publish tickers")
				}
			}
		}
	}
}

/*
measure records the latest quote volume for the ticking symbol and ranks it
against the live peer cross-section.
*/
func (signal *Signal) publishTickers(tickers []market.TickerUpdate) error {
	rows := make([]market.TickerUpdate, 0, len(tickers))

	for _, ticker := range tickers {
		if ticker.Last <= 0 {
			continue
		}

		signal.symbols.Store(ticker.Symbol, ticker.Volume*ticker.Last)
		rows = append(rows, ticker)
	}

	if len(rows) == 0 {
		return nil
	}

	volumes := signal.volumeSnapshot()
	tasks := make([]chan *qpool.QValue[any], 0, len(rows))

	for _, row := range rows {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			measurement, standout, err := signal.measureFromVolumes(row, volumes)

			if err != nil {
				return nil, err
			}

			if measurement.Symbol == "" {
				return nil, nil
			}

			if err := perspectives.AssignCategorySNR(
				&measurement, signal.floor, standout,
			); err != nil {
				return nil, err
			}

			activate.Once("signal/liquidity:measurement")
			signal.broadcasts["measurements"].Send(&qpool.QValue[any]{Value: measurement})

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)
	}

	return err
}

func (signal *Signal) measure(row market.TickerUpdate) (perspectives.Measurement, float64, error) {
	signal.symbols.Store(row.Symbol, row.Volume*row.Last)

	volumes := signal.volumeSnapshot()

	return signal.measureFromVolumes(row, volumes)
}

func (signal *Signal) measureFromVolumes(
	row market.TickerUpdate,
	volumes map[string]float64,
) (perspectives.Measurement, float64, error) {
	quoteVol := volumes[row.Symbol]

	peers := make([]float64, 0, len(volumes)-1)

	for symbol, volume := range volumes {
		if symbol == row.Symbol || volume <= 0 {
			continue
		}

		peers = append(peers, volume)
	}

	if quoteVol <= 0 || len(peers) < minLiquidityPeers {
		return perspectives.Measurement{}, 0, nil
	}

	median := numeric.PercentileSorted(numeric.CopySorted(peers), 0.5)

	if median <= 0 {
		return perspectives.Measurement{}, 0, fmt.Errorf(
			"liquidity: non-positive peer median for %s",
			row.Symbol,
		)
	}

	ratio := quoteVol / median
	raw := signal.strength(ratio)
	category, clarity, standout, err := liquidityReading(quoteVol, peers)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	trackedRaw, _ := signal.tracked.LoadOrStore(
		row.Symbol,
		perspectives.NewCategory(perspectives.CategoryTypeNone),
	)
	tracked := trackedRaw.(*perspectives.Category)

	confidence, err := tracked.Observe(category, clarity, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Symbol:     row.Symbol,
		Source:     perspectives.SourceLiquidity,
		Category:   category,
		Last:       row.Last,
		Strength:   raw,
		Confidence: confidence,
	}, standout, nil
}

func (signal *Signal) volumeSnapshot() map[string]float64 {
	volumes := make(map[string]float64)

	signal.symbols.Range(func(key, value any) bool {
		volumes[key.(string)] = value.(float64)

		return true
	})

	return volumes
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
