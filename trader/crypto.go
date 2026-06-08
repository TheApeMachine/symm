package trader

import (
	"context"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

type Crypto struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	ui     *qpool.BroadcastGroup
	bus    *internal.Bus
	desk   *broker.Desk
}

func NewCrypto(ctx context.Context, pool *qpool.Q[any]) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	return &Crypto{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		ui:     pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"raw", "ui"},
			[]string{"raw"},
		),
		desk: nil,
	}
}

/*
Tick consumes the raw bus. Story emits Action structs (entry/exit verdicts) and
the paper desk emits execution maps. Actions submit orders; executions update
inventory and the shared focus set so the not-holding gate sees open positions.
*/
func (crypto *Crypto) Tick() error {
	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			message, err := crypto.bus.Receive("raw")

			if err != nil {
				return err
			}

			if message == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			switch envelope["channel"] {
			case "actions":
				crypto.desk.AddOrder(envelope["action"].(logic.Action))
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	if crypto.cancel != nil {
		crypto.cancel()
	}

	return nil
}
