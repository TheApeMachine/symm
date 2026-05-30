package toxicity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Toxicity is the executed-flow book-quality service. It feeds the shared Tracker
from the public L2 book joined against the public trade tape, splitting
liquidity removals into fills vs cancels, and (via the package-level IsToxic)
lets the weighted-book readers in depthflow/fluid exclude toxic near-touch
walls. It is a service, not a perspectives source — it emits no measurements.
*/
type Toxicity struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	subscribers map[string]*qpool.Subscriber
	tracker     *Tracker
}

func NewToxicity(ctx context.Context, pool *qpool.Q) *Toxicity {
	ctx, cancel := context.WithCancel(ctx)

	tox := &Toxicity{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: make(map[string]*qpool.Subscriber),
		tracker:     Default(),
	}

	for _, channel := range []string{"trades", "tick", "book"} {
		group := pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		tox.subscribers[channel] = group.Subscribe("toxicity:"+channel, 128)
	}

	return tox
}

func (tox *Toxicity) Tick() error {
	var wg sync.WaitGroup

	wg.Go(func() { tox.consumeTrades() })
	wg.Go(func() { tox.consumeTicks() })
	wg.Go(func() { tox.consumeBook() })

	wg.Wait()
	return tox.ctx.Err()
}

func (tox *Toxicity) consumeTrades() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["trades"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity trades channel closed"))
				return
			}

			trades, tradesOK := value.Value.([]market.TradeUpdate)

			if !tradesOK {
				continue
			}

			for _, tick := range trades {
				tox.tracker.ObserveTrade(tick.Symbol, market.Pair{}, tick.Price, tick.Qty, tick.Timestamp)
			}
		}
	}
}

func (tox *Toxicity) consumeTicks() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["tick"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity tick channel closed"))
				return
			}

			row, rowOK := value.Value.(market.TickerUpdate)

			if !rowOK {
				continue
			}

			tox.tracker.ObserveMid(row.Symbol, market.Pair{}, midOf(row))
		}
	}
}

func (tox *Toxicity) consumeBook() {
	for {
		select {
		case <-tox.ctx.Done():
			return
		case value, ok := <-tox.subscribers["book"].Incoming:
			if !ok {
				errnie.Error(fmt.Errorf("toxicity book channel closed"))
				return
			}

			delta, deltaOK := value.Value.(market.BookUpdate)

			if !deltaOK {
				continue
			}

			now := time.Now()

			for _, level := range delta.Bids {
				tox.tracker.ApplyBookLevel(delta.Symbol, market.Pair{}, SideBid, level.Price, level.Qty, now)
			}

			for _, level := range delta.Asks {
				tox.tracker.ApplyBookLevel(delta.Symbol, market.Pair{}, SideAsk, level.Price, level.Qty, now)
			}
		}
	}
}

func (tox *Toxicity) Close() error {
	tox.cancel()
	return nil
}

func midOf(row market.TickerUpdate) float64 {
	if row.Bid > 0 && row.Ask > 0 {
		return (row.Bid + row.Ask) / 2
	}

	return row.Last
}
