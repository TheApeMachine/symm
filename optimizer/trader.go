package optimizer

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Trader is the tune-time stand-in for trader.Crypto: it routes live actions and
replays candidate trees to score configurations.
*/
type Trader struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	desk        *broker.Desk
}

func NewTrader(ctx context.Context, pool *qpool.Q) (*Trader, error) {
	ctx, cancel := context.WithCancel(ctx)

	desk, err := broker.NewDesk(ctx, pool)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("optimizer trader: desk: %w", err)
	}

	trader := &Trader{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		desk:        desk,
	}

	for _, channel := range []string{"actions"} {
		trader.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		trader.subscribers[channel] = trader.broadcasts[channel].Subscribe("optimizer:trader", 1024)
	}

	return trader, nil
}

func (trader *Trader) Tick() error {
	for {
		select {
		case <-trader.ctx.Done():
			return trader.ctx.Err()
		case row, ok := <-trader.subscribers["actions"].Incoming:
			if !ok {
				return nil
			}

			if row == nil {
				continue
			}

			action, ok := row.Value.(perspectives.Action)

			if !ok {
				return fmt.Errorf("optimizer trader: invalid action type %T", row.Value)
			}

			if err := trader.desk.AddOrder(action); err != nil {
				return fmt.Errorf("optimizer trader: order: %w", err)
			}
		}
	}
}

/*
Evaluate replays measurements through one branch registry and returns PnL.
*/
func (trader *Trader) Evaluate(
	branches perspectives.BranchList, rows []perspectives.Measurement,
) float64 {
	return NewReplaySimulation(trader.ctx, branches, rows).Score()
}

func (trader *Trader) Close() error {
	trader.cancel()

	return nil
}
