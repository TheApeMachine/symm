package broker

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/ui"
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
	rest      *network.REST
	price     *Price
	balance   *Balance
	history   *History
	positions *Cache
	virtuals  *Cache
	books     *Book
	settler   interface {
		Act(symbol, path, action string)
	}
}

/*
BindSettler connects the model so closed virtual positions settle through
the same Act path an open one entered on, keeping Wins/Promoted honest.
*/
func (desk *Desk) BindSettler(settler interface {
	Act(symbol, path, action string)
}) {
	desk.settler = settler
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance *Balance,
	store *store.Client,
) *Desk {
	desk := &Desk{
		transport: transport,
		rest:      network.NewREST(),
		balance:   balance,
		history:   NewHistory(ctx, store),
		price:     price,
		positions: NewCache(ctx),
		virtuals:  NewCache(ctx),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	for key, val := range map[string]*Cache{
		"position": desk.positions,
		"virtual":  desk.virtuals,
	} {
		for value := range desk.history.Restore(key) {
			var position *Position
			value.Lookup(position, key)

			if position == nil {
				continue
			}

			val.Restore(position)
		}
	}

	desk.Transition(runtime.READY)
	return desk
}

/*
AllPositions is every position the desk holds — open and closed, paper and
virtual — each with its live mark. The dashboard derives every position
readout (top bar, open lots, life-cycle, longtail) from this single frame,
so all books stay consistent with the desk.
*/
func (desk *Desk) AllPositions() []*Position {
	all := make([]*Position, 0)
	all = append(all, desk.positions.All()...)
	all = append(all, desk.virtuals.All()...)
	return all
}

/*
Hello is a callback for the ui.Hub that fires when a new WebSocket
connection is established, and we want to update the client with
some initial data.
*/
func (desk *Desk) Hello() {
	io.Copy(ui.UI, data.NewValue("positions", "update", "all").Append(
		desk.AllPositions(), "positions",
	))

	if desk.balance.value != nil {
		io.Copy(ui.UI, desk.balance.value)
	}
}

/*
Update marks open positions against the trade price and settles any virtual
whose band walk is over. Called from the workspace tick handler — the desk sits
off the trade ingress path, so this is where marks become live.
*/
func (desk *Desk) Update(trades ...*kraken.TradeData) {
	for _, val := range []*Cache{
		desk.positions, desk.virtuals,
	} {
		for _, trade := range trades {
			position := val.Get(trade.Symbol, true)

			if position == nil {
				continue
			}

			position.Update(&trade.Price)
		}
	}

	desk.SettleVirtuals()

	// One merged frame per tick: every position the desk holds, open and
	// closed, paper and virtual, each with its live mark. Every frontend
	// position readout derives from this single frame.
	io.Copy(ui.UI, data.NewValue("positions", "update", "all").Append(
		desk.AllPositions(), "positions",
	))
}

/*
SettleVirtuals closes every virtual whose band walk is over. Closing routes
through the bound settler (the model's Act "close") so the position is
stored closed and the path scores Wins/Promoted the same way an Act-driven
close would. With no settler it closes on the desk directly.
*/
func (desk *Desk) SettleVirtuals() {
	for _, position := range desk.virtuals.All(true) {
		if !position.ShouldExit() {
			continue
		}

		if desk.settler != nil && position.Path != "" {
			desk.settler.Act(position.Symbol, position.Path, "close")
			continue
		}

		if err := desk.Exit(position.Symbol, true); err != nil {
			desk.Error(err)
		}
	}
}

/*
Has reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Has(symbol string, virtual bool) bool {
	if virtual {
		return desk.virtuals.Has(symbol)
	}

	return desk.positions.Has(symbol)
}

/*
Apply an execution to the desk.
*/
func (desk *Desk) Apply(executions *kraken.Execution) {
	for _, execution := range executions.Data {
		position := desk.positions.Get(execution.Symbol, true)

		if position == nil {
			position = desk.virtuals.Get(execution.Symbol, true)
		}

		if position == nil {
			continue
		}

		position.Apply(execution)
	}
}

/*
Enter a new position.
*/
func (desk *Desk) Enter(
	symbol string, path string, virtual bool,
) error {
	if desk.positions.Has(symbol) || desk.virtuals.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[desk.Enter] position already exists",
			nil,
		)
	}

	position := NewPosition(symbol, path, virtual, desk.price)

	cash := desk.balance.cash(desk.balance.Quote)

	// The wallet lags the first ticks (private poll) and paper's quote asset
	// may not be named in the balance map; fall back to the configured bank-
	// roll so sizing never depends on the poll having landed.
	if cash == nil || cash.Sign() <= 0 {
		cash = desk.price.ReferenceCash()
	}

	// Allocation cap: never commit more than 20% of current cash.
	if cash != nil && cash.Sign() > 0 {
		cash = cash.Mul(decimal.NewFromFloat64(0.2))
	}

	allocation, err := desk.price.AllocateEntry(symbol, cash)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] unable to size entry for "+symbol,
			err,
		))
	}

	qty := allocation.Quantity

	if qty == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] no tradable quantity for "+symbol,
			nil,
		))
	}

	position.Qty = qty
	order := position.Enter()

	key := "position"

	if virtual {
		key = "virtual"
	}

	if err := desk.history.Store(data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	io.Copy(ui.UI, data.NewValue("positions", "update", "positions").Append(
		[]*Position{position}, "positions",
	))

	if err := desk.Send(order, virtual); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to send order",
			err,
		))
	}

	return nil
}

/*
Exit a position.
*/
func (desk *Desk) Exit(symbol string, virtual bool) error {
	var position *Position

	if virtual {
		position = desk.virtuals.Get(symbol, true)
	} else {
		position = desk.positions.Get(symbol, true)
	}

	if position == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Exit] position not found",
			nil,
		)
	}

	order := position.Exit()

	// Exit does not remove the position from the cache; it must move to the
	// closed side before any settlement reads or re-opens happen.
	if virtual {
		desk.virtuals.Remove(position.Symbol)
	} else {
		desk.positions.Remove(position.Symbol)
	}

	if virtual && position.price != nil {
		if cost, proceeds, profit, ret, ok := position.price.CloseOut(
			position.Symbol, position.Qty, position.EntryPrice, position.EntryFee,
		); ok {
			position.mu.Lock()

			position.Cost = cost
			position.Proceeds = proceeds
			position.Profit = profit

			net := ret * 100
			position.Return = &net
			position.NetReturn = &net
			position.OutcomeTriggers = []string{"band settle"}

			position.mu.Unlock()
		}
	}

	key := "position"

	if virtual {
		key = "virtual"
	}

	value := data.NewValue(
		key, "open", symbol,
	).Append(
		position, "position",
	)

	if err := desk.history.Move(value, strings.ReplaceAll(
		key, "open", "closed",
	)); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Enter] failed to store position",
			err,
		))
	}

	io.Copy(ui.UI, data.NewValue("positions", "update", "positions").Append(
		[]*Position{position}, "positions",
	))

	if err := desk.Send(order, virtual); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[desk.Exit] failed to send order",
			err,
		))
	}

	return nil
}

/*
Holding reports whether the desk holds an open position (paper or virtual)
for the symbol.
*/
func (desk *Desk) Holding(symbol string, virtual bool) bool {
	if virtual {
		return desk.virtuals.Has(symbol)
	}

	return desk.positions.Has(symbol)
}

/*
Profit reports whether the open position for the symbol is currently
profitable, based on the position's realized return. The second return
value is false when no open position exists for the symbol.
*/
func (desk *Desk) Profit(symbol string, virtual bool) (bool, bool) {
	var position *Position

	if virtual {
		position = desk.virtuals.Get(symbol, true)
	} else {
		position = desk.positions.Get(symbol, true)
	}

	if position == nil || position.Return == nil {
		return false, false
	}

	return *position.Return > 0, true
}

/*
send an order to the exchange.
*/
func (desk *Desk) Send(order *spot.OrderRequest, virtual bool) error {
	if virtual {
		return nil
	}

	buf, err := sonic.Marshal(order)

	if err != nil {
		return errnie.Err(
			errnie.IO,
			"[desk.Send] failed to marshal order",
			err,
		)
	}

	return desk.transport.Write(buf)
}
