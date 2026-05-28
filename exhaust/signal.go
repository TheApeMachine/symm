package exhaust

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

/*
Exhaust tracks book/trade microstructure decay and advises exit urgency.

mu is exposed so the per-channel goroutine independence test
(TestExhaustTickDrainsEachChannelGoroutine) can hold the lock and
demonstrate that the channel consumers continue to drain without
acquiring it. None of the production handlers take mu — it is a
documentation seam for that invariant. If a future change introduces
shared state that *should* be guarded, take mu inside the handler and
the test catches the regression.
*/
type Exhaust struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	history     *historyStore
}

func NewExhaust(ctx context.Context, pool *qpool.Q) *Exhaust {
	ctx, cancel := context.WithCancel(ctx)

	exhaust := &Exhaust{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		history:     newHistoryStore(),
	}

	for _, channel := range []string{"book", "trade", "tick"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		exhaust.subscribers[channel] = group.Subscribe("exhaust:"+channel, 128)
	}

	exhaust.broadcasts["exits"] = pool.CreateBroadcastGroup("exits", 10*time.Millisecond)

	return exhaust
}

func (exhaust *Exhaust) Start() error        { return nil }
func (exhaust *Exhaust) State() engine.State { return engine.READY }

func (exhaust *Exhaust) Tick() error {
	errnie.Info("starting exhaust tick")

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-exhaust.ctx.Done():
				return
			case value, ok := <-exhaust.subscribers["book"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("exhaust book channel closed"))
					return
				}

				delta, deltaOK := value.Value.(market.BookLevelsDelta)
				if !deltaOK {
					errnie.Error(fmt.Errorf("exhaust: invalid book payload: %T", value.Value))
					continue
				}

				bidDepth := 0.0
				askDepth := 0.0

				for _, level := range delta.Bids {
					bidDepth += level.Volume
				}

				for _, level := range delta.Asks {
					askDepth += level.Volume
				}

				spreadBPS := 0.0
				imbalance := 0.0

				if len(delta.Bids) > 0 && len(delta.Asks) > 0 {
					bid := delta.Bids[0].Price
					ask := delta.Asks[0].Price
					mid := (bid + ask) / 2

					if mid > 0 {
						spreadBPS = (ask - bid) / mid * 10000
					}

					total := delta.Bids[0].Volume + delta.Asks[0].Volume

					if total > 0 {
						imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
					}
				}

				exhaust.history.observe(
					delta.Symbol,
					bidDepth,
					askDepth,
					bidDepth+askDepth,
					spreadBPS,
					0,
					imbalance,
					0,
				)

				exhaust.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-exhaust.ctx.Done():
				return
			case value, ok := <-exhaust.subscribers["trade"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("exhaust trade channel closed"))
					return
				}

				tick, tickOK := value.Value.(trade.Data)
				if !tickOK {
					errnie.Error(fmt.Errorf("exhaust: invalid trade payload: %T", value.Value))
					continue
				}

				sign := -1.0

				if tick.Side == "buy" {
					sign = 1.0
				}

				exhaust.history.observe(
					tick.Symbol, 0, 0, 0, 0, sign, 0, tick.Price,
				)

				exhaust.publishPulse()
			}
		}
	})

	wg.Go(func() {
		for {
			select {
			case <-exhaust.ctx.Done():
				return
			case value, ok := <-exhaust.subscribers["tick"].Incoming:
				if !ok {
					errnie.Error(fmt.Errorf("exhaust tick channel closed"))
					return
				}

				row, rowOK := value.Value.(market.TickerRow)
				if !rowOK {
					errnie.Error(fmt.Errorf("exhaust: invalid ticker payload: %T", value.Value))
					continue
				}

				exhaust.history.observe(
					row.Symbol, 0, 0, 0, 0, 0, 0, row.Last,
				)

				exhaust.publishPulse()
			}
		}
	})

	wg.Wait()
	return exhaust.ctx.Err()
}

func (exhaust *Exhaust) publishPulse() {
	threshold := config.System.ExitUrgencyThreshold

	for _, symbol := range exhaust.history.symbols() {
		snapshot, ok := exhaust.history.snapshot(symbol)

		if !ok {
			continue
		}

		urgency, reason := exitScoreLong(snapshot)

		if urgency < threshold {
			continue
		}

		exhaust.broadcasts["exits"].Send(&qpool.QValue[any]{
			Value: engine.Exit{
				Symbol:  symbol,
				Urgency: urgency,
				Reason:  reason,
			},
		})
	}
}

func (exhaust *Exhaust) Close() error {
	exhaust.cancel()
	return nil
}
