package broker

import (
	"context"
	"io"
	"math"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	tx "github.com/theapemachine/symm/data/transport"
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
	mu             sync.Mutex
	transport      Transport
	rest           *network.REST
	price          *Price
	balance        *Balance
	history        *History
	positions      *sync.Map
	virtuals       *sync.Map
	closed         []*Position
	closedMu       sync.RWMutex
	books          *Book
	positionStore  *store.Client
	positionHub    io.Writer
	openPublished  map[string]string
	closedArchived map[string]struct{}
	onClosed       func(*Position) error
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

	for position := range tx.Next(desk.history) {
		desk.positions.Store(position.Scope, position)
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

	for _, cache := range []*sync.Map{desk.positions, desk.virtuals} {
		cache.Range(func(key, value any) bool {
			position := value.(*Position)
			desk.quote(position)
			positions = append(positions, position)
			return true
		})
	}

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

	desk.closedMu.Lock()
	desk.closed = make([]*Position, 0)
	desk.closedMu.Unlock()

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
Enter sizes an entry from the book and the matched edge and starts working it
in child market orders. The size is the smallest of: the exit capacity within
the slippage budget at its weakest over the last expected hold, the standard
deviation of signed traded volume over one expected hold (participation
within flow noise), and what the available cash buys at the budget's entry
limit. Every refusal is an error naming its reason.
*/
func (desk *Desk) Enter(symbol string, virtual bool) error {
	if system.Cfg == nil || system.Cfg.Risk == nil || system.Cfg.Risk.MaxFraction == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[desk] risk budget unavailable",
			nil,
		))
	}

	if desk == nil || desk.price == nil {
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

	order := position.Enter(
		symbol,
		qty,
	)
	position.Virtual = virtual

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
	if desk == nil || desk.price == nil || symbol == "" {
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
	)

	if !virtual {
		untyped, ok = desk.positions.Load(symbol)
	} else {
		untyped, ok = desk.virtuals.Load(symbol)
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

	return nil
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
