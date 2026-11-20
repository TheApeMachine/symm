package broker

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
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
		return nil, false
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
			// Page-1 failure is not an empty account — refuse restore so live
			// spot holdings are not left unmanaged / re-bought.
			return nil, false
		}

		page := kraken.NewTradesHistoryResult(raw)

		if page == nil {
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

	// Fold from fill sides first — QueryOrders is only needed to bind
	// EntryOrder on leftovers. Fetching every historical txid burned N/50
	// REST calls at boot for closed cycles that never need the order row.
	book := map[string]kraken.HistoryOrder{}

	steps := make([]orderStep, 0, len(ids))

	for _, id := range ids {
		rows := byOrder[id]

		if len(rows) == 0 {
			continue
		}

		side := rows[0].Type
		// First fill time — never opentm. Resting limits submit early and would
		// reorder cycles / stamp OpenedAt before the print that actually opened.
		at := rows[0].Time
		symbol := ledger.symbol(rows[0].Pair)

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

	pairs := slices.Collect(maps.Keys(grouped))
	slices.Sort(pairs)

	for _, pair := range pairs {
		holding, done := ledger.cycle(ctx, pair, grouped[pair], byOrder, book)
		closed = append(closed, done...)

		if holding != nil {
			open = append(open, holding)
		}
	}

	if ledger.orders != nil {
		ledger.bindOpenOrders(open)
	}

	return open, closed
}

/*
bindOpenOrders QueryOrders only the leftover opens' venue ids and stamps
EntryOrder. Closed cycles keep fill-derived VenueOrderIDs without the REST cost.
*/
func (ledger *Ledger) bindOpenOrders(open []*Position) {
	ids := make([]string, 0, len(open))
	seen := make(map[string]struct{}, len(open))

	for _, position := range open {
		if position == nil {
			continue
		}

		for _, id := range []string{position.EntryVenueOrderID, position.ExitVenueOrderID} {
			if id == "" {
				continue
			}

			if _, ok := seen[id]; ok {
				continue
			}

			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return
	}

	book := ledger.orders.Fetch(ids)

	for _, position := range open {
		if position == nil || position.EntryVenueOrderID == "" {
			continue
		}

		order, ok := book[position.EntryVenueOrderID]
		if !ok {
			continue
		}

		position.EntryOrder = order.Request()
	}
}

/*
held drops folded opens the spot wallet does not carry. Cycles that already
had an exit are closed in cycle — only unmatched leftovers are candidates.
Open Qty is the spendable wallet size for that base; venue qty_min judges it.
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

		if cash == nil || cash.Sign() <= 0 {
			continue
		}

		if min := ledger.qtyMin(position.Symbol); min != nil &&
			cash.LessThan(*min) {
			continue
		}

		qty := *cash
		// Wallet size is source of truth for leftover qty. Reseed fills so
		// Apply's fillsNet matches inventory. Cost basis stays this leftover's
		// Entry — a prior closed cycle must never supply Entry/OpenedAt here.
		px := decimal.Zero
		if position.Entry != nil {
			px = *position.Entry
		}
		if px.Sign() <= 0 {
			continue
		}
		position.PnL = nil
		position.PnLPct = nil
		position.Stoploss = nil
		position.ExitPath = ""
		position.ExitTrigger = ""
		position.ExitVenueOrderID = ""
		position.ExitOrder = nil
		seed := qty
		position.Fills = []*market.ExecutionData{{
			Side:      "buy",
			LastQty:   &seed,
			LastPrice: &px,
			ExecID:    "wallet:" + position.Symbol,
			Timestamp: position.OpenedAt,
		}}
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

		// Exit fills close the cycle when almost all size is sold. A prior
		// closed round must never stay "open" and hand its Entry to a later buy.
		if step.side == "sell" && current.ClosedAt == nil {
			closedAt := step.at
			if last := fills[len(fills)-1].Time; !last.IsZero() {
				closedAt = last
			}
			ledger.finishClosedCycle(current, closedAt)
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

/*
finishClosedCycle stamps ClosedAt when an exit has effectively flattened the
cycle (under qty_min, or ≤1% of buys left from fee/rounding). Historical
buy+sell rounds stay closed; the next buy opens a new position.
*/
func (ledger *Ledger) finishClosedCycle(position *Position, at time.Time) {
	if position == nil {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ClosedAt != nil || position.Qty == nil || position.Qty.Sign() <= 0 {
		return
	}

	if !ledger.exitFlattenedLocked(position) {
		return
	}

	zero := decimal.Zero
	position.Qty = &zero
	closedAt := at
	position.ClosedAt = &closedAt
	position.Transition(runtime.DONE)
	position.stampFillRealizedLocked()
}

/*
exitFlattenedLocked is true when remaining qty after an exit is not an open
hold: under qty_min, or ≤1% of cycle buys. Caller holds position.mu.
*/
func (ledger *Ledger) exitFlattenedLocked(position *Position) bool {
	rem := *position.Qty
	if rem.Sign() <= 0 {
		return true
	}

	if min := ledger.qtyMin(position.Symbol); min != nil && rem.LessThan(*min) {
		return true
	}

	_, bought, _ := position.fillsNet()
	if bought.Sign() <= 0 {
		return false
	}

	return rem.Div(bought).LessThanOrEqual(decimal.NewFromFloat(0.01))
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
