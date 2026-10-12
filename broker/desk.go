package broker

import (
	"context"
	"slices"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
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
	mu        sync.Mutex
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

func (desk *Desk) ResetPaper() error {
	if paper, ok := desk.transport.(*Paper); ok {
		if err := paper.Reset(); err != nil {
			return errnie.Error(err)
		}

		if desk.balance != nil {
			return desk.balance.Update()
		}

		return nil
	}

	if err := ResetPaperAccount(desk.Context()); err != nil {
		return errnie.Error(err)
	}

	if desk.balance != nil {
		return desk.balance.Update()
	}

	return nil
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
	order := position.Enter(symbol, qty)

	desk.mu.Lock()
	untyped, loaded := desk.positions.Load(symbol)
	if loaded && untyped != nil {
		positions := untyped.([]*Position)
		desk.positions.Store(symbol, append(positions, position))
	}

	if !loaded || untyped == nil {
		desk.positions.Store(symbol, []*Position{position})
	}
	desk.mu.Unlock()

	return desk.submit(symbol, order)
}

/*
Holds reports whether the symbol has a position on the desk: open, entering,
or with its exit still working.
*/
func (desk *Desk) Holds(symbol string) bool {
	_, exists := desk.positions.Load(symbol)
	return exists
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. A position whose sell is already working is not sold twice.
*/
func (desk *Desk) Exit(symbol string) error {
	desk.mu.Lock()
	untyped, exists := desk.positions.Load(symbol)
	if !exists || untyped == nil {
		desk.mu.Unlock()
		return nil
	}

	positions := untyped.([]*Position)
	posList := slices.Clone(positions)
	desk.mu.Unlock()

	for _, position := range posList {
		if err := desk.submit(symbol, position.Exit(symbol)); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
submit writes an order to the venue transport. A nil order (nothing to do)
or an absent transport submits nothing.
*/
func (desk *Desk) submit(symbol string, order *spot.OrderRequest) error {
	if desk.transport == nil || order == nil {
		return nil
	}

	buf, err := sonic.Marshal(kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    symbol,
		Type:    order.Type,
		Volume:  order.Volume,
		ClOrdId: order.ClOrdId,
	}))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal, "[desk] unable to encode "+order.Type+" order for "+symbol, err,
		))
	}

	if err := desk.transport.Write(buf); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO, "[desk] unable to submit "+order.Type+" order for "+symbol, err,
		))
	}

	return nil
}

func (desk *Desk) Apply(execution *kraken.Execution) {
	for _, data := range execution.Data {
		desk.mu.Lock()
		untyped, exists := desk.positions.Load(data.Symbol)

		if !exists {
			desk.mu.Unlock()
			continue
		}

		positions := untyped.([]*Position)
		allClosed := true

		for _, position := range positions {
			position.Apply(data)

			if !position.Closed() {
				allClosed = false
			}
		}

		if allClosed {
			desk.positions.Delete(data.Symbol)
		}
		desk.mu.Unlock()
	}
}
