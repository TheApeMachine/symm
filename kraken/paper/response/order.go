package response

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

/*
Orders simulates Kraken private order methods. Taker orders fill through
broker.SlippageFill after one-way latency; post-only limits rest with L2 queue
position and fill when aggressor trades deplete queue ahead via broker.MakerQueueState.
Protective orders REST until price breaches, then use the same fill helpers.
*/
type Orders struct {
	ctx           context.Context
	pool          *qpool.Q
	quotes        *broker.QuoteCache
	balances      *Balances
	catalog       *PairCatalog
	latency       LatencySampler
	ids           *Identifier
	triggers      map[string]*restingTrigger
	makers        map[string]*restingMaker
	pendingTakers []pendingTaker
	observers     []types.Socket
	mu            sync.Mutex
}

// restingTrigger is a protective order the exchange (here, the emulator) holds until
// price breaches it. level is the fixed price for stop/take; trailing stops carry an
// offset fraction and advance peak from the market price at placement.
type restingTrigger struct {
	action  reasoning.ActionType
	qty     float64
	level   float64
	offset  float64
	peak    float64
	clOrdID string
	orderID string
	params  trading.AddParams
}

func NewOrders(ctx context.Context, pool *qpool.Q, balances *Balances, ids *Identifier) *Orders {
	return NewOrdersWithQuoteCache(
		ctx, pool, balances, ids, broker.EnsureQuoteCache(ctx, pool), nil, ZeroLatency(),
	)
}

/*
NewOrdersWithQuoteCache builds the paper order emulator against explicit quotes.
*/
func NewOrdersWithQuoteCache(
	ctx context.Context,
	pool *qpool.Q,
	balances *Balances,
	ids *Identifier,
	quotes *broker.QuoteCache,
	catalog *PairCatalog,
	latency LatencySampler,
) *Orders {
	if latency == nil {
		latency = ZeroLatency()
	}

	orders := &Orders{
		ctx:      ctx,
		pool:     pool,
		quotes:   quotes,
		balances: balances,
		catalog:  catalog,
		latency:  latency,
		ids:      ids,
		triggers: make(map[string]*restingTrigger),
		makers:   make(map[string]*restingMaker),
	}

	if quotes != nil {
		quotes.SubscribeTrades(orders.onTrade)
	}

	return orders
}

func (orders *Orders) Send(message *qpool.QValue[any]) map[string]any {
	frame, ok := message.Value.(map[string]any)

	if !ok {
		return nil
	}

	out := map[string]any{
		"method":   frame["method"],
		"req_id":   frame["req_id"],
		"success":  true,
		"time_in":  frame["time_in"],
		"time_out": time.Now(),
	}

	if frame["method"] != trading.MethodAddOrder {
		return out
	}

	params, ok := frame["params"].(trading.AddParams)

	if !ok {
		return out
	}

	orderID := orders.ids.OrderID()
	out["result"] = map[string]any{
		"order_id":  orderID,
		"cl_ord_id": params.ClOrdID,
	}

	action := reasoning.ActionFromOrderType(params.OrderType)

	if reasoning.IsProtectiveExit(action) {
		orders.armTrigger(params, action, orderID)
		return out
	}

	orders.routeFill(params, orderID)

	return out
}

func (orders *Orders) routeFill(params trading.AddParams, orderID string) {
	if params.OrderType == trading.Limit && params.PostOnly {
		orders.armMaker(params, orderID)
		return
	}

	orders.scheduleTaker(params, orderID)
}

/*
armTrigger holds a protective order in the trigger book instead of filling it. One
trigger per symbol — the most recent protective gate wins, matching the replay
ledger so a strategy can tighten its stop on a later tick. No wallet is debited until
the trigger fires (the exchange holds the order, it has not executed).
*/
func (orders *Orders) armTrigger(
	params trading.AddParams,
	action reasoning.ActionType,
	orderID string,
) {
	trigger := &restingTrigger{
		action:  action,
		qty:     params.OrderQty,
		clOrdID: params.ClOrdID,
		orderID: orderID,
		params:  params,
	}

	if params.Triggers != nil {
		if reasoning.IsTrailingExit(action) {
			trigger.offset = -params.Triggers.Price / 100

			if quote, ok := orders.quotes.Snapshot(params.Symbol); ok {
				trigger.peak = quote.Last
			}
		} else {
			trigger.level = params.Triggers.Price
		}
	}

	orders.mu.Lock()
	orders.triggers[params.Symbol] = trigger
	orders.mu.Unlock()

	orders.notifyArm(params, orderID)
}

/*
CheckTriggers polls every resting protective order against the latest quote,
advancing the trailing peak and filling when price breaches the level. The websocket
calls it each tick — the paper emulation of Kraken's server-side trigger engine.
*/
func (orders *Orders) CheckTriggers() {
	orders.mu.Lock()
	symbols := make([]string, 0, len(orders.triggers))

	for symbol := range orders.triggers {
		symbols = append(symbols, symbol)
	}

	orders.mu.Unlock()

	var breached []string

	for _, symbol := range symbols {
		orders.mu.Lock()
		trigger := orders.triggers[symbol]
		orders.mu.Unlock()

		if trigger == nil {
			continue
		}

		quote, ok := orders.quotes.Snapshot(symbol)

		if !ok || quote.Last <= 0 {
			continue
		}

		level := trigger.level

		if reasoning.IsTrailingExit(trigger.action) {
			if quote.Last > trigger.peak {
				trigger.peak = quote.Last
			}

			level = reasoning.ProtectiveLevel(trigger.action, 0, trigger.peak, trigger.offset)
		}

		if reasoning.ProtectiveBreached(trigger.action, level, quote.Last) {
			orders.closeAtTrigger(symbol, trigger, level, quote.Last)
			breached = append(breached, symbol)
		}
	}

	if len(breached) == 0 {
		return
	}

	orders.mu.Lock()

	for _, symbol := range breached {
		delete(orders.triggers, symbol)
	}

	orders.mu.Unlock()
}

/*
closeAtTrigger realizes a breached protective exit. The -limit variants rest as
maker orders and fill at their trigger level (maker fee, no crossing slippage); the
market variants cross the book via broker.SlippageFill — a stop or trail eats a
downside gap-through — paying the taker fee. Mirrors optimizer/replay/ledger.go.
*/
func (orders *Orders) closeAtTrigger(
	symbol string,
	trigger *restingTrigger,
	level, last float64,
) {
	maker := reasoning.ExitRestsAsLimit(trigger.action)

	if maker {
		fee := orders.feeAmount(symbol, trigger.qty, level, true)
		orders.notifyFill(FillNotice{
			Params:       trigger.params,
			OrderID:      trigger.orderID,
			Price:        level,
			Fee:          fee,
			LiquidityInd: "m",
			Maker:        true,
		})

		return
	}

	price, err := orders.takerFillPrice(
		symbol, trading.Sell, trigger.qty, level, trigger.action,
	)

	if err != nil {
		orders.notifyFill(FillNotice{
			Params: trigger.params, OrderID: trigger.orderID, Reason: err.Error(),
		})

		return
	}

	fee := orders.feeAmount(symbol, trigger.qty, price, false)

	orders.notifyFill(FillNotice{
		Params:       trigger.params,
		OrderID:      trigger.orderID,
		Price:        price,
		Fee:          fee,
		LiquidityInd: "t",
		Maker:        false,
	})
}

func (orders *Orders) notifyFill(notice FillNotice) {
	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{Type: noticeFill, Value: notice})
	}
}

func (orders *Orders) notifyArm(params trading.AddParams, orderID string) {
	for _, observer := range orders.observers {
		observer.Send(&qpool.QValue[any]{
			Type: noticeArm,
			Value: ArmNotice{
				Params:  params,
				OrderID: orderID,
			},
		})
	}
}

func (orders *Orders) Observe(socket types.Socket) {
	orders.observers = append(orders.observers, socket)
}
