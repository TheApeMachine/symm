package broker

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
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
	positions *sync.Map
	virtuals  *sync.Map
	closed    []*Position
	closedMu  sync.RWMutex
	books     *Book
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
		positions: &sync.Map{},
		virtuals:  &sync.Map{},
		closed:    make([]*Position, 0),
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)

	for position, err := range desk.history.Restore() {
		if err != nil {
			desk.Error(err)
			continue
		}

		if position.Virtual {
			desk.virtuals.Store(position.Symbol, position)
			continue
		}

		desk.positions.Store(position.Symbol, position)
	}

	desk.Transition(runtime.READY)
	return desk
}

func (desk *Desk) Balance() []byte {
	if desk == nil || desk.balance == nil {
		return nil
	}

	wallet := desk.balance.Wallet()

	if wallet == nil {
		return nil
	}

	account := struct {
		Data       []kraken.BalanceData `json:"data"`
		Unrealized *decimal.Decimal     `json:"n,omitempty"`
		Equity     *decimal.Decimal     `json:"e,omitempty"`
	}{Data: wallet.Data}

	if trade := desk.balance.Trade(); trade != nil {
		account.Unrealized = trade.UnrealizedPnL
		account.Equity = trade.Equity
	}

	raw, err := sonic.Marshal(account)

	if err != nil {
		desk.Error(err)
		return nil
	}

	return raw
}

func (desk *Desk) Positions() []byte {
	positions := make([]*Position, 0)
	seen := make(map[string]struct{})

	for _, cache := range []*sync.Map{desk.positions, desk.virtuals} {
		if cache == nil {
			continue
		}
		cache.Range(func(key, value any) bool {
			position := value.(*Position)
			desk.quote(position)
			positions = append(positions, position)
			if position.ID != "" {
				seen[position.ID] = struct{}{}
			}
			return true
		})
	}

	desk.closedMu.RLock()
	for _, position := range desk.closed {
		if position == nil {
			continue
		}
		if position.ID != "" {
			if _, ok := seen[position.ID]; ok {
				continue
			}
			seen[position.ID] = struct{}{}
		}
		positions = append(positions, position)
	}
	desk.closedMu.RUnlock()

	json, err := sonic.Marshal(positions)

	if err != nil {
		desk.Error(err)
		return nil
	}

	return json
}

/*
quote writes the prices the open-position card reads. A lot opened before
its fill landed takes the ask it can see, copied so a later book update
does not move the entry. The mark and the return come from Price.Sale.
Exit room is how many times the visible bids cover the quantity.
*/
func (desk *Desk) quote(position *Position) {
	if desk == nil || desk.price == nil || position == nil {
		return
	}

	position.mu.Lock()
	symbol := position.Symbol
	qty := position.Qty
	entry := position.EntryPrice
	fee := position.EntryFee
	position.mu.Unlock()

	if symbol == "" || qty == nil || qty.Sign() <= 0 {
		return
	}

	bid, ask := desk.price.Touch(symbol)

	if (entry == nil || entry.Sign() <= 0) && ask != nil && ask.Sign() > 0 {
		entry = ask.Copy()
	}

	if fee == nil && entry != nil {
		if rate, ok := desk.price.FeeRate(symbol); ok {
			fee = notional(entry, qty).Mul(decimal.NewFromFloat64(rate))
		}
	}

	var mark *decimal.Decimal
	var ret *float64

	if sale, _, fraction, ok := desk.price.Sale(symbol, qty, entry, fee); ok {
		mark = decimal.NewFromFloat64(sale)
		ret = &fraction
	} else if bid != nil && bid.Sign() > 0 {
		mark = bid.Copy()
	}

	var room *float64
	var vwap *decimal.Decimal

	if desk.price.Books != nil {
		if surface, err := desk.price.SellQuote(symbol, qty); err == nil &&
			surface != nil &&
			surface.BookComplete &&
			surface.ExecutableQty != nil &&
			surface.ExecutableQty.Sign() > 0 {
			value := surface.ExecutableQty.Div(qty).Float64()
			room = &value

			if surface.FullyExecutable && surface.ExecutableVWAP != nil {
				vwap = surface.ExecutableVWAP
			}
		}
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if (position.EntryPrice == nil || position.EntryPrice.Sign() <= 0) && entry != nil {
		position.EntryPrice = entry
	}

	if position.EntryFee == nil && fee != nil {
		position.EntryFee = fee
	}

	if mark != nil {
		position.Mark = mark
	}

	position.Return = ret
	position.ExitRoom = room

	if vwap != nil {
		position.ExitVWAP = vwap
	}
}

func (desk *Desk) Cash(asset string) *decimal.Decimal {
	return desk.balance.cash(asset)
}

func (desk *Desk) ResetPaper() error {
	desk.mu.Lock()
	desk.positions = &sync.Map{}
	desk.mu.Unlock()

	desk.virtuals = &sync.Map{}

	desk.closedMu.Lock()
	desk.closed = make([]*Position, 0)
	desk.closedMu.Unlock()

	if err := desk.history.Drop(); err != nil {
		return errnie.Error(err)
	}

	if paper, ok := desk.transport.(*Paper); ok {
		if err := paper.Reset(); err != nil {
			return errnie.Error(err)
		}

		return nil
	}

	if err := ResetPaperAccount(desk.Context()); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Holding reports whether desk carries a lot for the symbol on the book asked (paper or virtual).
*/
func (desk *Desk) Holding(symbol string, virtual bool) bool {
	if desk == nil || symbol == "" {
		return false
	}

	book := desk.positions

	if virtual {
		book = desk.virtuals
	}

	if book == nil {
		return false
	}

	_, ok := book.Load(symbol)
	return ok
}

/*
Enter sizes an entry from the book and works it in a market order. The size
is the smallest of: what the cash budget (risk.max_fraction of current cash,
the maximum allocation per entry) buys at the entry touch, and the quantity
the visible bids can absorb, so every entry leaves room to sell the position
off. Every refusal is an error naming its reason.
*/
func (desk *Desk) Enter(symbol string, virtual bool) error {
	if desk == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] no desk",
			nil,
		))
	}

	if system.Cfg == nil || system.Cfg.Risk == nil || system.Cfg.Risk.MaxFraction == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] risk budget unavailable",
			nil,
		))
	}

	if desk.price == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] price unavailable",
			nil,
		))
	}

	var ok bool

	if !virtual {
		_, ok = desk.positions.Load(symbol)
	} else {
		_, ok = desk.virtuals.Load(symbol)
	}

	if ok {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[desk] position already exists for "+symbol,
			nil,
		))
	}

	position := NewPosition()
	position.EntryTick = data.Tick.Load()

	var cash *decimal.Decimal

	if virtual {
		if system.Cfg.Market == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[desk] virtual wallet unavailable",
				nil,
			))
		}

		cash = decimal.NewFromInt64(int64(system.Cfg.Market.Balance))
	} else {
		if desk.balance == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[desk] balance unavailable",
				nil,
			))
		}

		cash = desk.balance.cash(desk.balance.Quote)
	}

	points := int64(math.Round(system.Cfg.Risk.MaxFraction.Float64() * 10_000))

	budget := cash.Mul(decimal.NewFromInt64(points)).Div(
		decimal.NewFromInt64(10_000),
	)

	qty, err := desk.price.Quantity(symbol, budget)

	if err != nil {
		return errnie.Error(err)
	}

	// Book-aware sizing: never enter more than the visible bids can absorb,
	// so every entry leaves room to sell the position off.
	if surface, err := desk.price.SellQuote(symbol, qty); err == nil &&
		surface != nil &&
		surface.BookComplete &&
		surface.ExecutableQty != nil &&
		surface.ExecutableQty.Sign() > 0 &&
		surface.ExecutableQty.Cmp(qty) < 0 {
		bid, ask := desk.price.Touch(symbol)

		if bid == nil || bid.Sign() <= 0 {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"[desk] no bid to size the exit cushion for "+symbol,
				nil,
			))
		}

		cushion, err := desk.price.Affordable(symbol, budget, ask)

		if err != nil {
			return errnie.Error(err)
		}

		if cushion == nil || cushion.Cmp(surface.ExecutableQty) > 0 {
			qty = surface.ExecutableQty
		} else {
			qty = cushion
		}

		if !desk.price.Tradable(symbol, qty, ask) {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"[desk] exit cushion below the minimum order for "+symbol,
				nil,
			))
		}
	}

	order := position.Enter(
		symbol,
		qty,
	)
	position.Virtual = virtual

	/*
		Both books persist before anything else can happen: the lot must exist
		on disk before a venue call, so a crash between here and the fill replay
		never loses an open position. A virtual lot is desk-side emulated: it is
		priced from the book on the spot and enters immediately.
	*/
	if virtual {
		if bid, _ := desk.price.Touch(symbol); bid != nil && bid.Sign() > 0 {
			position.EntryPrice = bid.Copy()

			if rate, ok := desk.price.FeeRate(symbol); ok {
				position.EntryFee = notional(bid, qty).Mul(
					decimal.NewFromFloat64(rate),
				)
			}
		}
	}

	if err := desk.history.Record(position); err != nil {
		return errnie.Error(err)
	}

	if !virtual {
		if _, err := desk.rest.Post(
			"https://api.kraken.com/0/private/AddOrder",
			order,
		); err != nil {
			return errnie.Error(err)
		}

		desk.positions.Store(symbol, position)
		return nil
	}

	desk.virtuals.Store(symbol, position)
	return nil
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. A position whose sell is already working is not sold twice.
*/
/*
Profit reports whether closing the lot now would clear its cost. The sale
comes from the book. ok is false when the lot or the book cannot price it.
*/
func (desk *Desk) Profit(symbol string, virtual bool) (bool, bool) {
	if desk == nil || symbol == "" {
		return false, false
	}

	var (
		untyped any
		ok      bool
	)

	if virtual {
		untyped, ok = desk.virtuals.Load(symbol)
	} else {
		untyped, ok = desk.positions.Load(symbol)
	}

	if !ok {
		return false, false
	}

	position := untyped.(*Position)
	position.mu.Lock()
	qty := position.Qty
	entry := position.EntryPrice
	fee := position.EntryFee
	position.mu.Unlock()

	if (entry == nil || entry.Sign() <= 0) && desk.price != nil {
		if _, ask := desk.price.Touch(symbol); ask != nil && ask.Sign() > 0 {
			entry = ask.Copy()
		}
	}

	_, _, _, ret, priced := desk.price.CloseOut(symbol, qty, entry, fee)

	if !priced {
		return false, false
	}

	return ret > 0, true
}

func (desk *Desk) Exit(symbol string, virtual bool) error {
	var (
		untyped any
		ok      bool
		cache   *sync.Map
		book    = "paper"
	)

	if !virtual {
		untyped, ok = desk.positions.Load(symbol)
		cache = desk.positions
	} else {
		untyped, ok = desk.virtuals.Load(symbol)
		cache = desk.virtuals
		book = "virtual"
	}

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"[desk] position not found for "+symbol,
			nil,
		))
	}

	position := untyped.(*Position)
	order := position.Exit(symbol)

	if !virtual {
		if _, err := desk.rest.Post(
			"https://api.kraken.com/0/private/AddOrder",
			order,
		); err != nil {
			return errnie.Error(err)
		}
	}

	/*
		The exit is final: score the round trip from the book, retire the lot
		from the open book, and drop its open object so a restart never restores
		a closed position. A failed persistence is an error — the exit already
		happened on the venue, so it is loud, not silent.
	*/
	desk.settle(position, book)

	cache.Delete(symbol)

	if err := desk.history.Forget(symbol); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
settle closes one lot: the close time, the outcome priced from the book
(never zero when it cannot be priced — the triggers say why), the hold, and
a copy into the closed list the Positions frame carries.
*/
func (desk *Desk) settle(position *Position, book string) {
	position.mu.Lock()

	if position.ClosedAt == nil {
		now := time.Now().UTC()
		position.ClosedAt = &now
	}

	position.ExitTick = data.Tick.Load()
	position.Virtual = book == "virtual"

	closedAt := position.ClosedAt
	openedAt := position.OpenedAt
	position.mu.Unlock()

	if !openedAt.IsZero() {
		hold := closedAt.Sub(openedAt).Nanoseconds()
		position.mu.Lock()
		position.HoldNs = hold
		position.mu.Unlock()
	}

	var (
		cost, proceeds, profit *decimal.Decimal
		ret                    float64
		ok                     bool
	)

	if desk.price != nil {
		cost, proceeds, profit, ret, ok = desk.price.CloseOut(
			position.Symbol, position.Qty, position.EntryPrice, position.EntryFee,
		)
	}

	position.mu.Lock()

	if ok {
		position.Cost = cost
		position.Proceeds = proceeds
		position.Profit = profit
		position.NetReturn = &ret
		position.OutcomeTriggers = []string{}
	} else {
		position.OutcomeTriggers = []string{"unpriced"}
	}

	position.mu.Unlock()

	desk.closedMu.Lock()

	for _, existing := range desk.closed {
		if existing.ID == position.ID {
			desk.closedMu.Unlock()
			return
		}
	}

	desk.closed = append(desk.closed, position)
	desk.closedMu.Unlock()
}

func (desk *Desk) Apply(execution *kraken.Execution) {
	for _, data := range execution.Data {
		for _, cache := range []*sync.Map{desk.positions, desk.virtuals} {
			untyped, ok := cache.Load(data.Symbol)

			if !ok {
				continue
			}

			position := untyped.(*Position)
			position.Apply(data)
		}
	}
}
