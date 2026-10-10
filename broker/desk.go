package broker

import (
	"context"
	"sync"

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

	untyped, _ := desk.positions.LoadOrStore(symbol, []*Position{position})
	positions := untyped.([]*Position)
	desk.positions.Store(symbol, append(positions, position))

	return nil
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. When the position is already exiting (the monitor's full exit came
first) it records that it also fired, so both are visible.
*/
func (desk *Desk) Exit(symbol string) error {
	untyped, _ := desk.positions.Load(symbol)
	positions := untyped.([]*Position)

	for _, position := range positions {
		position.Exit(symbol)
	}

	return nil
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
