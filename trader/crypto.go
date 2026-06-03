package trader

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/activate"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/market/perspectives"
)

const cryptoRawSubscriberID = "trader/crypto:raw"

/*
Crypto publishes wallet snapshots to the ui broadcast from Kraken balance frames.
*/
type Crypto struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	ui            *qpool.BroadcastGroup
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	desk          *broker.Desk
	streams       *focus.Set
	pendingOrders sync.Map
	auditSeq      atomic.Int64
	balanceOnce   sync.Once
	cash          float64
	inventory     map[string]float64
	avgEntry      map[string]float64
	marks         map[string]float64
}

func NewCrypto(ctx context.Context, pool *qpool.Q, streams *focus.Set) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	desk, err := broker.NewDesk(ctx, pool)

	if err != nil {
		cancel()
		errnie.Error(fmt.Errorf("trader/crypto: desk: %w", err), "trader/crypto")
		return nil
	}

	crypto := &Crypto{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		ui:          pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		streams:     streams,
		desk:        desk,
		inventory:   make(map[string]float64),
		avgEntry:    make(map[string]float64),
		marks:       make(map[string]float64),
	}

	for _, channel := range []string{"raw"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(channel, 1024)
	}

	activate.Boot("trader/crypto ready")

	return crypto
}

func (crypto *Crypto) Tick() error {
	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case message := <-crypto.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			envelope, ok := message.Value.(map[string]any)

			if !ok {
				continue
			}

			channel, _ := envelope["channel"].(string)
			switch channel {
			case "actions":
				action, ok := message.Value.(perspectives.Action)

				if !ok {
					continue
				}

				switch action.Type {
				case perspectives.ActionLimit:
					crypto.desk.AddOrder(action)
				case perspectives.ActionMarket:
					crypto.desk.AddOrder(action)
				case perspectives.ActionIceberg:
					crypto.desk.AddOrder(action)
				case perspectives.ActionStopLoss:
					crypto.desk.AddOrder(action)
				case perspectives.ActionStopLossLimit:
					crypto.desk.AddOrder(action)
				case perspectives.ActionTakeProfit:
					crypto.desk.AddOrder(action)
				case perspectives.ActionTakeProfitLimit:
					crypto.desk.AddOrder(action)
				case perspectives.ActionTrailingStop:
					crypto.desk.AddOrder(action)
				case perspectives.ActionTrailingStopLimit:
					crypto.desk.AddOrder(action)
				case perspectives.ActionSettlePosition:
					crypto.desk.AddOrder(action)
				}
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	return nil
}
