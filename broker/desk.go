package broker

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Desk owns open positions: it sizes entries from the book (exit capacity
within the slippage budget, flow noise and cash), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	transport Transport
	price     *Price
	balance   *Balance
	positions *sync.Map
	books     *Book
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
) *Desk {
	desk := &Desk{
		transport: transport,
		balance:   balance,
		price:     price,
		positions: &sync.Map{},
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	desk.Transition(runtime.READY)
	return desk
}

func (desk *Desk) Cash(asset string) *decimal.Decimal {
	if desk.balance == nil {
		if desk.price != nil {
			return desk.price.ReferenceCash()
		}

		return nil
	}

	return desk.balance.cash(asset)
}

/*
Enter sizes an entry from the book and the matched edge and starts working it
in child market orders. The size is the smallest of: the exit capacity within
the slippage budget at its weakest over the last expected hold, the standard
deviation of signed traded volume over one expected hold (participation
within flow noise), and what the available cash buys at the budget's entry
limit. Every refusal is an error naming its reason.
*/
func (desk *Desk) Enter(symbol string, qty *decimal.Decimal) error {
	position := NewPosition()
	position.Enter(symbol, qty)

	untyped, loaded := desk.positions.LoadOrStore(symbol, []*Position{position})
	if loaded {
		positions := untyped.([]*Position)
		desk.positions.Store(symbol, append(positions, position))
	}

	if desk.transport != nil && len(position.entryOrders) > 0 {
		order := position.entryOrders[len(position.entryOrders)-1]
		msg := kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
			Pair:    symbol,
			Type:    "buy",
			Volume:  qty.String(),
			ClOrdId: order.ClOrdId,
		})
		buf, err := sonic.Marshal(msg)

		if err == nil {
			desk.transport.Write(buf)
		}
	}

	return nil
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. When the position is already exiting (the monitor's full exit came
first) it records that it also fired, so both are visible.
*/
func (desk *Desk) Exit(symbol string) error {
	untyped, exists := desk.positions.Load(symbol)
	if !exists || untyped == nil {
		return nil
	}

	positions := untyped.([]*Position)

	for _, position := range positions {
		position.Exit(symbol)

		if desk.transport != nil && position.qty != nil && len(position.exitOrders) > 0 {
			order := position.exitOrders[len(position.exitOrders)-1]
			msg := kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
				Pair:    symbol,
				Type:    "sell",
				Volume:  position.qty.String(),
				ClOrdId: order.ClOrdId,
			})
			buf, err := sonic.Marshal(msg)

			if err == nil {
				desk.transport.Write(buf)
			}
		}
	}

	desk.positions.Delete(symbol)
	return nil
}

/*
Has reports whether there are open positions for symbol.
*/
func (desk *Desk) Has(symbol string) bool {
	if desk == nil || desk.positions == nil {
		return false
	}

	untyped, exists := desk.positions.Load(symbol)
	if !exists || untyped == nil {
		return false
	}

	positions, ok := untyped.([]*Position)
	return ok && len(positions) > 0
}

func (desk *Desk) Apply(execution *kraken.Execution) {
	for _, data := range execution.Data {
		untyped, exists := desk.positions.Load(data.Symbol)

		if !exists {
			continue
		}

		positions := untyped.([]*Position)

		for _, position := range positions {
			position.Apply(data)
		}
	}
}
