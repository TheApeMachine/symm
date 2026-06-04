package trader

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
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
	pending       map[string]struct{}
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
		pending:     make(map[string]struct{}),
	}

	for _, channel := range []string{"raw"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(cryptoRawSubscriberID, 1024)
	}

	errnie.Info("trader/crypto ready", "trader/crypto")

	return crypto
}

/*
Tick consumes the raw bus. Story emits Action structs (entry/exit verdicts) and
the paper desk emits execution maps. Actions submit orders; executions update
inventory and the shared focus set so the not-holding gate sees open positions.
*/
func (crypto *Crypto) Tick() error {
	incoming := crypto.subscribers["raw"].Incoming

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case message := <-incoming:
			if message == nil || message.Value == nil {
				continue
			}

			if action, ok := message.Value.(perspectives.Action); ok {
				crypto.submit(action)
				continue
			}

			if envelope, ok := message.Value.(map[string]any); ok {
				crypto.observeExecution(envelope)
			}
		}
	}
}

/*
submit forwards an entry or exit verdict to the desk. One in-flight order per
symbol is allowed: entries only when flat, exits only when holding, and never
while an order for that symbol is still resolving. This is race-free because
Tick processes actions and executions on a single goroutine. Exit verdicts carry
no quantity, so we settle the full position we currently hold.
*/
func (crypto *Crypto) submit(action perspectives.Action) {
	if action.Type == perspectives.ActionNone {
		return
	}

	if _, inFlight := crypto.pending[action.Symbol]; inFlight {
		return
	}

	held := crypto.inventory[action.Symbol]

	if perspectives.IsEntryAction(action.Type) && held > 0 {
		return
	}

	if perspectives.IsExitAction(action.Type) {
		if held <= 0 {
			return
		}

		action.Quantity = held
	}

	// A desk rejection is the gate working as intended, not an in-flight order.
	if err := crypto.desk.AddOrder(action); err != nil {
		return
	}

	crypto.pending[action.Symbol] = struct{}{}
}

/*
observeExecution applies a paper fill to inventory and the focus set, clearing
the in-flight marker. A buy opens or adds to a position; a settling sell closes
it. A zero-quantity execution is a no-fill: it only clears the in-flight marker.
*/
func (crypto *Crypto) observeExecution(envelope map[string]any) {
	if envelope["channel"] != "executions" {
		return
	}

	symbol, _ := envelope["symbol"].(string)

	if symbol == "" {
		return
	}

	delete(crypto.pending, symbol)

	side, _ := envelope["side"].(string)
	qty, _ := envelope["qty"].(float64)
	price, _ := envelope["price"].(float64)

	if qty <= 0 {
		return
	}

	if side == string(trading.Buy) {
		crypto.openPosition(symbol, qty, price)
		return
	}

	crypto.closePosition(symbol)
}

func (crypto *Crypto) openPosition(symbol string, qty, price float64) {
	prevQty := crypto.inventory[symbol]
	prevCost := prevQty * crypto.avgEntry[symbol]
	newQty := prevQty + qty

	crypto.inventory[symbol] = newQty
	crypto.avgEntry[symbol] = (prevCost + qty*price) / newQty
	crypto.streams.Add(symbol)
	crypto.publishPositions()
}

func (crypto *Crypto) closePosition(symbol string) {
	delete(crypto.inventory, symbol)
	delete(crypto.avgEntry, symbol)
	crypto.streams.Remove(symbol)
	crypto.publishPositions()
}

/*
publishPositions ships the open book to the dashboard so it can mark each
position against the live price stream and show real-time unrealized P&L.
*/
func (crypto *Crypto) publishPositions() {
	if crypto.ui == nil {
		return
	}

	positions := make([]map[string]any, 0, len(crypto.inventory))

	for symbol, qty := range crypto.inventory {
		if qty <= 0 {
			continue
		}

		positions = append(positions, map[string]any{
			"symbol":    symbol,
			"qty":       qty,
			"avg_entry": crypto.avgEntry[symbol],
		})
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "positions",
		"positions": positions,
	}})
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	return nil
}
