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

	crypto.broadcasts["raw"] = pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	crypto.subscribers["raw"] = crypto.broadcasts["raw"].Subscribe(
		cryptoRawSubscriberID, 1024,
	)

	crypto.subscribers["ui:resync"] = pool.CreateBroadcastGroup(
		"ui:resync", 10*time.Millisecond,
	).Subscribe("trader/crypto:resync", 1024)

	activate.Boot("trader/crypto ready")

	return crypto
}

func (crypto *Crypto) Tick() error {
	if err := crypto.ensureBalanceSnapshot(); err != nil {
		return err
	}

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case _, ok := <-crypto.subscribers["ui:resync"].Incoming:
			if !ok {
				return crypto.ctx.Err()
			}

			if err := crypto.requestBalanceSnapshot(); err != nil {
				return err
			}

			if err := crypto.resendWallet(); err != nil {
				return err
			}
		case message := <-crypto.subscribers["raw"].Incoming:
			if err := crypto.handleRaw(message); err != nil {
				return err
			}
		}
	}
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	return nil
}
