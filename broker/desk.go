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
		Holds(symbol string, virtual bool) bool
	}
}

/*
BindSettler connects the model so closed virtual positions settle through
the same Act path an open one entered on, keeping Wins/Promoted honest.
*/
func (desk *Desk) BindSettler(settler interface {
	Act(symbol, path, action string)
	Holds(symbol string, virtual bool) bool
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
			position := &Position{}
			value.Lookup(position, key)

			if value.Err != nil {
				// A frame that does not decode is already recorded on the
				// value; report it and keep restoring the rest.
				desk.Error(value.Err)
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
/*
forget retires a closed position's row from its cache. The history value was
already relocated by Move; the desk keeps the row on the closed side only
until settlement finishes with it.
*/
func (desk *Desk) forget(position *Position) {
	cache := desk.positions

	if position.Virtual {
		cache = desk.virtuals
	}

	cache.Drop(position.Symbol, position.ID)
}

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
func (desk *Desk) OnTrade(trades []kraken.TradeData) {
	for _, val := range []*Cache{
		desk.positions, desk.virtuals,
	} {
		for i := range trades {
			position := val.Get(trades[i].Symbol, true)

			if position == nil {
				continue
			}

			position.Update(&trades[i].Price)
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

A virtual the model does not hold open (restored from history without a
policy entry) has nothing to score: it closes on the desk directly.
*/
func (desk *Desk) SettleVirtuals() {
	for _, position := range desk.virtuals.All(true) {
		if !position.ShouldExit() {
			continue
		}

		if desk.settler != nil && position.Path != "" &&
			desk.settler.Holds(position.Symbol, true) {
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
			position = desk.positions.Get(execution.Symbol, false)
		}

		if position == nil {
			position = desk.virtuals.Get(execution.Symbol, true)
		}

		if position == nil {
			continue
		}

		position.Apply(execution)

		// A paper lot whose sell filled is fully settled: retire it from
		// the desk so it cannot shadow the next position on the symbol.
		if !position.Virtual && position.ClosedAt != nil {
			desk.forget(position)
		}
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

	if desk.price.Status() != runtime.READY {
		// Boot ordering, not a fault: sizing waits for fees and reference
		// cash, so the tick is skipped silently instead of flooding the log.
		return nil
	}

	cash := desk.balance.cash(desk.balance.Quote)

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

	order, err := position.Enter()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk.Enter] unable to enter "+symbol,
			err,
		))
	}

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

	// Settle before anything moves: the outcome is final the moment the
	// exit is recorded.
	if virtual && position.price != nil {
		cost, proceeds, profit, ret, ok := position.price.CloseOut(
			position.Symbol, position.Qty, position.EntryPrice, position.EntryFee,
		)

		if !ok {
			// No price to close out against: keep the virtual open and
			// retry on the next settlement pass.
			position.mu.Lock()
			position.ClosedAt = nil
			position.HoldNs = 0
			position.mu.Unlock()

			return errnie.Err(
				errnie.UnprocessableContent,
				"[desk.Exit] no price to close out "+position.Symbol,
				nil,
			)
		}

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

	// The exit is final: the position leaves the open cache. A paper
	// position stays addressable on the closed side until the venue's sell
	// executions land (Apply settles the lot wherever the position lives),
	// then history.Move retires it from the desk entirely.
	if virtual {
		desk.virtuals.Remove(position.Symbol)
	} else {
		desk.positions.Remove(position.Symbol)
		defer desk.forget(position)
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
