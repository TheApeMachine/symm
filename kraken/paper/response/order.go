package response

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Orders simulates Kraken private order methods. Ordinary orders (entries, market
exits) fill immediately against the live quote cache. Protective orders (stop /
take-profit / trailing-stop) instead REST: they are held in a per-symbol trigger
book and filled only when a later quote breaches their level — exactly what the live
Kraken exchange does server-side, so the desk/trader path is identical and switching
to the authenticated socket is a no-op. The breach math is perspectives.* so paper,
the replay scorer, and the desk cannot drift.
*/
type Orders struct {
	ctx       context.Context
	pool      *qpool.Q
	quotes    *broker.QuoteCache
	balances  *Balances
	raw       *qpool.BroadcastGroup
	triggers  map[string]*restingTrigger
	observers []types.Socket
}

// restingTrigger is a protective order the exchange (here, the emulator) holds until
// price breaches it. level is the fixed price for stop/take; trailing stops carry an
// offset fraction and advance peak from the market price at placement.
type restingTrigger struct {
	action  perspectives.ActionType
	qty     float64
	level   float64
	offset  float64
	peak    float64
	clOrdID string
}

func NewOrders(ctx context.Context, pool *qpool.Q, balances *Balances) *Orders {
	return NewOrdersWithQuoteCache(ctx, pool, balances, broker.EnsureQuoteCache(ctx, pool))
}

/*
NewOrdersWithQuoteCache builds the paper order emulator against explicit quotes.
*/
func NewOrdersWithQuoteCache(
	ctx context.Context,
	pool *qpool.Q,
	balances *Balances,
	quotes *broker.QuoteCache,
) *Orders {
	return &Orders{
		ctx:      ctx,
		pool:     pool,
		quotes:   quotes,
		balances: balances,
		raw:      pool.CreateBroadcastGroup("raw", 10*time.Millisecond),
		triggers: make(map[string]*restingTrigger),
	}
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

	if frame["method"] == trading.MethodAddOrder {
		if params, ok := frame["params"].(trading.AddParams); ok {
			if action := perspectives.ActionFromOrderType(params.OrderType); perspectives.IsProtectiveExit(action) {
				orders.armTrigger(params, action)
			} else {
				orders.fill(params)
			}

			out["result"] = params
		}
	}

	return out
}

/*
armTrigger holds a protective order in the trigger book instead of filling it. One
trigger per symbol — the most recent protective gate wins, matching the replay
ledger so a strategy can tighten its stop on a later tick. No wallet is debited until
the trigger fires (the exchange holds the order, it has not executed).
*/
func (orders *Orders) armTrigger(params trading.AddParams, action perspectives.ActionType) {
	trigger := &restingTrigger{
		action:  action,
		qty:     params.OrderQty,
		clOrdID: params.ClOrdID,
	}

	if params.Triggers != nil {
		if perspectives.IsTrailingExit(action) {
			// The desk sends a negative-percent offset; Kraken (and we) trail from
			// the market price at placement.
			trigger.offset = -params.Triggers.Price / 100

			if quote, ok := orders.quotes.Snapshot(params.Symbol); ok {
				trigger.peak = quote.Last
			}
		} else {
			trigger.level = params.Triggers.Price
		}
	}

	orders.triggers[params.Symbol] = trigger
}

/*
CheckTriggers polls every resting protective order against the latest quote,
advancing the trailing peak and filling when price breaches the level. The websocket
calls it each tick — the paper emulation of Kraken's server-side trigger engine.
*/
func (orders *Orders) CheckTriggers() {
	var breached []string

	for symbol, trigger := range orders.triggers {
		quote, ok := orders.quotes.Snapshot(symbol)

		if !ok || quote.Last <= 0 {
			continue
		}

		level := trigger.level

		if perspectives.IsTrailingExit(trigger.action) {
			// Peak bootstraps from the first real quote (zero never fires), then
			// ratchets up — Kraken trails from the market price at placement.
			if quote.Last > trigger.peak {
				trigger.peak = quote.Last
			}

			level = perspectives.ProtectiveLevel(trigger.action, 0, trigger.peak, trigger.offset)
		}

		if perspectives.ProtectiveBreached(trigger.action, level, quote.Last) {
			orders.closeAtTrigger(symbol, trigger, level, quote.Last)
			breached = append(breached, symbol)
		}
	}

	for _, symbol := range breached {
		delete(orders.triggers, symbol)
	}
}

/*
closeAtTrigger realizes a breached protective exit. The -limit variants rest as
maker orders and fill at their trigger level (maker fee, no crossing slippage); the
market variants cross the book — a stop or trail eats a downside gap-through — paying
the taker fee and slippage. Mirrors optimizer/replay/ledger.go closeAtTrigger.
*/
func (orders *Orders) closeAtTrigger(symbol string, trigger *restingTrigger, level, last float64) {
	var fill, feePct float64

	if perspectives.ExitRestsAsLimit(trigger.action) {
		fill = level
		feePct = viper.GetFloat64("trading.paper.maker_fee_pct")
	} else {
		crossed := level

		if trigger.action != perspectives.ActionTakeProfit && last < level {
			crossed = last
		}

		slip := viper.GetFloat64("trading.paper.slippage_bps") / 10000
		fill = crossed * (1 - slip)
		feePct = viper.GetFloat64("trading.paper.taker_fee_pct")
	}

	fee := trigger.qty * fill * feePct / 100

	if orders.balances != nil {
		if err := orders.balances.ApplyFill(
			symbol, string(trading.Sell), trigger.qty, fill, fee, trigger.clOrdID,
		); err != nil {
			orders.emit(symbol, string(trading.Sell), 0, 0, 0, err.Error())

			return
		}
	}

	orders.emit(symbol, string(trading.Sell), trigger.qty, fill, fee, "trigger")
}

/*
fill prices an accepted order, settles it against the paper wallet, and emits the
resulting execution on raw.
*/
func (orders *Orders) fill(params trading.AddParams) {
	price, fee, err := orders.priceAndFee(params)

	if err != nil {
		errnie.Error(err)
		// Emit a no-fill execution so the trader clears its in-flight marker and
		// the dashboard can report why the order never filled.
		orders.emit(params.Symbol, string(params.Side), 0, 0, 0, err.Error())

		return
	}

	if orders.balances != nil {
		if err := orders.balances.ApplyFill(
			params.Symbol, string(params.Side), params.OrderQty, price, fee, params.ClOrdID,
		); err != nil {
			// Insufficient funds: the exchange rejects, nothing changes.
			// Emit a no-fill carrying the reason so the dashboard can show it.
			orders.emit(params.Symbol, string(params.Side), 0, 0, 0, err.Error())

			return
		}
	}

	// A discretionary/market exit closes the position; cancel any resting protective
	// trigger so it cannot fire later on a phantom position.
	if params.Side == trading.Sell {
		delete(orders.triggers, params.Symbol)
	}

	orders.emit(params.Symbol, string(params.Side), params.OrderQty, price, fee, "")
}

func (orders *Orders) emit(symbol, side string, qty, price, fee float64, reason string) {
	orders.raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": "executions",
		"symbol":  symbol,
		"side":    side,
		"qty":     qty,
		"price":   price,
		"fee":     fee,
		"reason":  reason,
	}})
}

/*
priceAndFee derives the fill price and fee from the single live quote, faithful
to maker/taker mechanics so no phantom edge is manufactured from a stale price
source. A maker (limit) order joins the touch on its own side — a buy fills at
the bid, a sell at the ask — and pays the maker fee. A taker (market) order
crosses the spread plus slippage and pays the taker fee. A maker-in/taker-out
round trip therefore loses the spread plus fees on a flat market and only profits
when the price genuinely moves while the position is held.
*/
func (orders *Orders) priceAndFee(params trading.AddParams) (price, fee float64, err error) {
	quote, ok := orders.quotes.Snapshot(params.Symbol)

	if !ok {
		return 0, 0, fmt.Errorf("paper fill: no quote for %s", params.Symbol)
	}

	slip := viper.GetFloat64("trading.paper.slippage_bps") / 10000
	maker := params.OrderType == trading.Limit

	switch {
	case params.Side == trading.Buy && maker:
		price = quote.Bid
	case params.Side == trading.Buy:
		price = quote.Ask * (1 + slip)
	case maker:
		price = quote.Ask
	default:
		price = quote.Bid * (1 - slip)
	}

	feePct := viper.GetFloat64("trading.paper.taker_fee_pct")

	if maker {
		feePct = viper.GetFloat64("trading.paper.maker_fee_pct")
	}

	return price, params.OrderQty * price * feePct / 100, nil
}

func (orders *Orders) Observe(socket types.Socket) {
	orders.observers = append(orders.observers, socket)
}
