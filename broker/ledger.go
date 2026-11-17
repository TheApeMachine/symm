package broker

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
Ledger restores holdings the same way live trading works: discover ordertxids
from TradesHistory, QueryOrders for those ids, open a position on each buy
order and close it on sells until qty flats, then Apply the fills that belong
to those orders. Incomplete history refuses opens. Still-open leftovers also
need a spot Balance for that base.
*/
type Ledger struct {
	tierKey   string
	price     *Price
	transport network.Client
	orders    *Orders
	onSettled func(*Position)
}

/*
NewLedger wires the collaborators a restored position needs to mark and exit.
*/
func NewLedger(
	tierKey string,
	price *Price,
	transport network.Client,
	onSettled func(*Position),
) *Ledger {
	return &Ledger{
		tierKey:   tierKey,
		price:     price,
		transport: transport,
		orders:    NewOrders(transport),
		onSettled: onSettled,
	}
}

/*
Restore pages trades history, QueryOrders for each ordertxid, folds order
cycles into holdings, then keeps only opens the wallet still holds.
*/
func (ledger *Ledger) Restore(ctx context.Context) (open []*Position, closed []*Position) {
	if ledger == nil {
		return nil, nil
	}

	fills, complete := ledger.history()

	if !complete {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.ledger] incomplete trades history; skipping restore",
			nil,
		))

		return nil, nil
	}

	open, closed = ledger.Fold(ctx, fills)
	open = ledger.held(open)

	return open, closed
}

/*
history pages TradesHistory until the cursor ends. complete is false when a
transport or venue error stops the walk after any page was accepted — callers
must not Fold that partial slice into opens.
*/
func (ledger *Ledger) history() (fills []kraken.HistoryTrade, complete bool) {
	if ledger == nil || ledger.transport == nil {
		return nil, true
	}

	seen := make(map[string]struct{})
	fills = make([]kraken.HistoryTrade, 0)
	cursor := ""

	for {
		req := kraken.NewTradesHistoryRequest(nil)
		req.Cursor = cursor

		raw, err := ledger.transport.Post(
			system.Cfg.WebSocket.Endpoints.TradesHistory,
			req,
		)

		if err != nil {
			errnie.Error(err)

			if len(fills) == 0 {
				return nil, true
			}

			return nil, false
		}

		page := kraken.NewTradesHistoryResult(raw)

		if page == nil {
			if len(fills) == 0 {
				return nil, true
			}

			return nil, false
		}

		for _, trade := range page.Fills() {
			if trade.ID != "" {
				if _, ok := seen[trade.ID]; ok {
					continue
				}

				seen[trade.ID] = struct{}{}
			}

			fills = append(fills, trade)
		}

		if page.Cursor.Next == "" || page.Cursor.Next == cursor {
			return fills, true
		}

		cursor = page.Cursor.Next
	}
}

/*
Fold QueryOrders each ordertxid, then walks those orders in time per pair.
A buy order opens a holding; sell orders peel it; flat closes that cycle and
the next buy starts another. Fills Apply only after the order is bound — same
order→fill seam as live Enter/Exit. Pairs outside the instrument universe are
skipped.
*/
func (ledger *Ledger) Fold(
	ctx context.Context, trades []kraken.HistoryTrade,
) (open []*Position, closed []*Position) {
	if ledger == nil {
		return nil, nil
	}

	byOrder := make(map[string][]kraken.HistoryTrade)
	ids := make([]string, 0)

	for _, trade := range trades {
		if trade.Pair == "" || trade.Volume == nil || trade.Volume.Sign() <= 0 {
			continue
		}

		symbol := ledger.symbol(trade.Pair)

		if !ledger.tradable(symbol) {
			continue
		}

		if trade.OrderID == "" {
			continue
		}

		if _, ok := byOrder[trade.OrderID]; !ok {
			ids = append(ids, trade.OrderID)
		}

		byOrder[trade.OrderID] = append(byOrder[trade.OrderID], trade)
	}

	for id, rows := range byOrder {
		slices.SortFunc(rows, func(a, b kraken.HistoryTrade) int {
			if c := a.Time.Compare(b.Time); c != 0 {
				return c
			}

			return strings.Compare(a.ID, b.ID)
		})
		byOrder[id] = rows
	}

	book := map[string]kraken.HistoryOrder{}

	if ledger.orders != nil {
		book = ledger.orders.Fetch(ids)
	}

	steps := make([]orderStep, 0, len(ids))

	for _, id := range ids {
		rows := byOrder[id]

		if len(rows) == 0 {
			continue
		}

		side := rows[0].Type
		at := rows[0].Time
		symbol := ledger.symbol(rows[0].Pair)

		if order, ok := book[id]; ok {
			if order.Descr.Type != "" {
				side = order.Descr.Type
			}

			if order.Descr.Pair != "" {
				symbol = ledger.symbol(order.Descr.Pair)
			}

			if tm := openTm(order); !tm.IsZero() {
				at = tm
			}
		}

		if !ledger.tradable(symbol) {
			continue
		}

		steps = append(steps, orderStep{id: id, symbol: symbol, side: side, at: at})
	}

	slices.SortFunc(steps, func(a, b orderStep) int {
		if c := a.at.Compare(b.at); c != 0 {
			return c
		}

		return strings.Compare(a.id, b.id)
	})

	grouped := make(map[string][]orderStep)

	for _, s := range steps {
		grouped[s.symbol] = append(grouped[s.symbol], s)
	}

	for pair, rows := range grouped {
		holding, done := ledger.cycle(ctx, pair, rows, byOrder, book)
		closed = append(closed, done...)

		if holding != nil {
			open = append(open, holding)
		}
	}

	return open, closed
}

/*
held drops folded opens the spot wallet does not carry. History leftovers after
a flat wallet are not positions. Open Qty is the spendable wallet size for that
base — folded net can be fee-dust while the wallet still holds a real pile
(DOGE/XDG is the usual case), and venue qty_min must judge the wallet, not the
history residue.
*/
func (ledger *Ledger) held(open []*Position) []*Position {
	if len(open) == 0 {
		return open
	}

	wallet, ok := ledger.balances()

	if !ok || wallet == nil {
		errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.ledger] balance unavailable; skipping open admission",
			nil,
		))

		return nil
	}

	kept := make([]*Position, 0, len(open))

	for _, position := range open {
		if position == nil {
			continue
		}

		cash := ledger.cash(wallet, position.Symbol)

		if cash == nil {
			continue
		}

		if min := ledger.qtyMin(position.Symbol); min != nil &&
			cash.LessThan(*min) {
			continue
		}

		qty := *cash
		position.Qty = &qty
		kept = append(kept, position)
	}

	return kept
}

/*
cash is spendable spot for the pair's base. Instrument base, normalizer Name,
and raw strip aliases are tried so XXDG/XDG/DOGE all hit the same wallet row.
*/
func (ledger *Ledger) cash(
	wallet *kraken.Balance, symbol string,
) *decimal.Decimal {
	if wallet == nil {
		return nil
	}

	for _, asset := range ledger.bases(symbol) {
		if cash := wallet.AssetCash(asset); cash != nil {
			return cash
		}
	}

	return nil
}

func (ledger *Ledger) bases(symbol string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 3)

	add := func(asset string) {
		if asset == "" {
			return
		}

		if _, ok := seen[asset]; ok {
			return
		}

		seen[asset] = struct{}{}
		out = append(out, asset)
	}

	base := ledger.base(symbol)
	add(base)

	if ledger.price != nil && ledger.price.normalizer != nil {
		add(ledger.price.normalizer.Name(base))
	}

	return out
}

func (ledger *Ledger) balances() (*kraken.Balance, bool) {
	if ledger == nil || ledger.transport == nil {
		return nil, false
	}

	frame := kraken.NewBalance()

	if ledger.price != nil && ledger.price.normalizer != nil {
		frame.Use(ledger.price.normalizer)
	}

	raw, err := ledger.transport.Post(
		system.Cfg.WebSocket.Endpoints.Balance,
		frame.Request(),
	)

	if err != nil {
		errnie.Error(err)
		return nil, false
	}

	if err := sonic.Unmarshal(raw, frame); err != nil {
		errnie.Error(err)
		return nil, false
	}

	return frame, true
}

func (ledger *Ledger) base(symbol string) string {
	if ledger.price != nil && ledger.price.Instrument != nil {
		if pair := ledger.price.Instrument.Pair(symbol); pair.Base != "" {
			return pair.Base
		}
	}

	quote := ""

	if system.Cfg != nil && system.Cfg.Market != nil {
		quote = strings.ToUpper(system.Cfg.Market.QuoteCurrency)
	}

	name := strings.ToUpper(strings.ReplaceAll(symbol, "/", ""))

	if quote != "" && strings.HasSuffix(name, quote) {
		return strings.TrimSuffix(name, quote)
	}

	return name
}

func (ledger *Ledger) qtyMin(symbol string) *decimal.Decimal {
	if ledger.price == nil || ledger.price.Instrument == nil {
		return nil
	}

	return ledger.price.Instrument.Pair(symbol).QtyMin
}

func (ledger *Ledger) tradable(symbol string) bool {
	if ledger.price == nil || ledger.price.Instrument == nil {
		return true
	}

	return ledger.price.Instrument.Has(symbol)
}

type orderStep struct {
	id     string
	symbol string
	side   string
	at     time.Time
}

func (ledger *Ledger) cycle(
	ctx context.Context,
	pair string,
	rows []orderStep,
	byOrder map[string][]kraken.HistoryTrade,
	book map[string]kraken.HistoryOrder,
) (*Position, []*Position) {
	var current *Position
	closed := make([]*Position, 0)

	for _, step := range rows {
		fills := byOrder[step.id]

		if len(fills) == 0 {
			continue
		}

		order, hasOrder := book[step.id]

		if current == nil || current.ClosedAt != nil {
			if step.side != "buy" {
				continue
			}

			current = NewPosition(
				ctx, pair, ledger.tierKey, "", ledger.price, ledger.transport, ledger.onSettled,
			)
			current.OpenedAt = step.at
			current.EntryVenueOrderID = step.id

			if hasOrder {
				current.EntryOrder = order.Request()
			}
		} else if step.side == "sell" {
			current.ExitVenueOrderID = step.id

			if hasOrder {
				current.ExitOrder = order.Request()
			}
		}

		for _, trade := range fills {
			current.Apply(ledger.execution(pair, trade))
		}

		if current.ClosedAt != nil {
			if current.ExitVenueOrderID == "" {
				current.ExitVenueOrderID = step.id
			}

			closed = append(closed, current)
		}
	}

	if current == nil || current.ClosedAt != nil ||
		current.Qty == nil || current.Qty.Sign() <= 0 {
		return nil, closed
	}

	// Still open: never keep a partial-sell ExitOrder — it must not hitch onto
	// the next live exit.
	current.ExitVenueOrderID = ""
	current.ExitOrder = nil

	return current, closed
}

func (ledger *Ledger) symbol(pair string) string {
	if ledger.price != nil && ledger.price.normalizer != nil {
		if name := ledger.price.normalizer.Name(pair); name != "" {
			return name
		}
	}

	return pair
}

func (ledger *Ledger) execution(symbol string, trade kraken.HistoryTrade) market.ExecutionData {
	return market.ExecutionData{
		OrderID:     trade.OrderID,
		ExecID:      trade.ID,
		ExecType:    "trade",
		Symbol:      symbol,
		Side:        trade.Type,
		LastQty:     trade.Volume,
		LastPrice:   trade.Price,
		Cost:        trade.Cost,
		OrderType:   trade.OrderType,
		Timestamp:   trade.Time,
		CumQty:      trade.Volume,
		CumCost:     trade.Cost,
		AvgPrice:    trade.Price,
		FeeUsdEquiv: trade.Fee,
		OrderStatus: "filled",
	}
}
