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
)

/*
Orders simulates Kraken private order methods. Accepted orders fill immediately
against the live quote cache (limit at its resting price, market at the touch
plus configured slippage), debit/credit the paper wallet, and emit an execution
frame on raw so the trader can track inventory and holding state.
*/
type Orders struct {
	ctx       context.Context
	pool      *qpool.Q
	quotes    *broker.QuoteCache
	balances  *Balances
	raw       *qpool.BroadcastGroup
	observers []types.Socket
}

func NewOrders(ctx context.Context, pool *qpool.Q, balances *Balances) *Orders {
	return &Orders{
		ctx:      ctx,
		pool:     pool,
		quotes:   broker.EnsureQuoteCache(ctx, pool),
		balances: balances,
		raw:      pool.CreateBroadcastGroup("raw", 10*time.Millisecond),
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
			orders.fill(params)
			out["result"] = params
		}
	}

	return out
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
