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
		// Emit a no-fill execution so the trader clears its in-flight marker.
		orders.emit(params.Symbol, string(params.Side), 0, 0, 0)

		return
	}

	if orders.balances != nil {
		if err := orders.balances.ApplyFill(
			params.Symbol, string(params.Side), params.OrderQty, price, fee, params.ClOrdID,
		); err != nil {
			// Insufficient funds: the exchange rejects, nothing changes.
			// Emit a no-fill so the trader clears its in-flight marker.
			orders.emit(params.Symbol, string(params.Side), 0, 0, 0)

			return
		}
	}

	orders.emit(params.Symbol, string(params.Side), params.OrderQty, price, fee)
}

func (orders *Orders) emit(symbol, side string, qty, price, fee float64) {
	orders.raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": "executions",
		"symbol":  symbol,
		"side":    side,
		"qty":     qty,
		"price":   price,
		"fee":     fee,
	}})
}

/*
priceAndFee derives the fill price and fee from configured paper costs. Limit
orders rest and fill at their own price as makers; market orders cross the touch
plus slippage as takers.
*/
func (orders *Orders) priceAndFee(params trading.AddParams) (price, fee float64, err error) {
	slipBps := viper.GetFloat64("trading.paper.slippage_bps")

	if params.OrderType == trading.Limit {
		makerPct := viper.GetFloat64("trading.paper.maker_fee_pct")
		notional := params.OrderQty * params.LimitPrice

		return params.LimitPrice, notional * makerPct / 100, nil
	}

	quote, ok := orders.quotes.Snapshot(params.Symbol)

	if !ok {
		return 0, 0, fmt.Errorf("paper fill: no quote for %s", params.Symbol)
	}

	price = quote.Ask * (1 + slipBps/10000)

	if params.Side == trading.Sell {
		price = quote.Bid * (1 - slipBps/10000)
	}

	takerPct := viper.GetFloat64("trading.paper.taker_fee_pct")

	return price, params.OrderQty * price * takerPct / 100, nil
}

func (orders *Orders) Observe(socket types.Socket) {
	orders.observers = append(orders.observers, socket)
}
