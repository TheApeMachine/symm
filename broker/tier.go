package broker

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
TierType selects transport and unlock rules — same Price book, different whether
orders leave the process and whether paper must prove edge first.
*/
type TierType uint8

const (
	TierTypeVirtual TierType = iota
	TierTypePaper
	TierTypeReal
)

func TierString(key string) TierType {
	switch key {
	case "virtual":
		return TierTypeVirtual
	case "paper":
		return TierTypePaper
	case "real":
		return TierTypeReal
	}

	return TierTypeVirtual
}

/*
String is the cache tierKey and hub JSON label; invalid values log because
corrupt tier keys would write positions under the wrong S3 prefix.
*/
func (tt TierType) String() string {
	switch tt {
	case TierTypeVirtual:
		return "virtual"
	case TierTypePaper:
		return "paper"
	case TierTypeReal:
		return "real"
	}

	errnie.Error(errnie.Err(
		errnie.NotFound,
		"[broker.TierType] invalid tier type",
		nil,
	))

	return ""
}

/*
Tier is one parallel book of positions sharing Price but not cache or socket —
paper proof unlocks disarmed real without mixing fills across lanes.
*/
type Tier struct {
	*runtime.System
	ctx        context.Context
	Type       TierType
	public     network.Client
	private    network.Client
	price      *Price
	venue      market.Venue
	positions  *Cache
	pending    *sync.Map
	balance    *market.Balance
	normalizer *spot.Normalizer
	onBook     func()
	// reserved maps symbol → fee-gross quote spend parked between Allocate and
	// fill/reject so the next Enter cannot reuse the same stale Balance cash.
	reserved sync.Map
}

/*
NewTier wires cache, REST balance polls, and private socket open. Frame handling
lives in Run → ingest — UnmarshalJSON only decodes.
*/
func NewTier(
	ctx context.Context,
	tt TierType,
	public network.Client,
	private network.Client,
	price *Price,
	host market.Venue,
	client *store.Client,
) *Tier {
	tier := &Tier{
		System:  runtime.NewSystem(ctx, "tier:"+tt.String()),
		ctx:     ctx,
		Type:    tt,
		public:  public,
		private: private,
		price:   price,
		venue:   host,
		pending: &sync.Map{},
		balance: market.NewBalance(map[string]any{
			"quote":  system.Cfg.Market.QuoteCurrency,
			"assets": []*market.Asset{},
		}),
		normalizer: price.normalizer,
	}

	tier.positions = NewCache(
		ctx, tt.String(), private, price, tier.settle, client,
	)

	for _, position := range tier.positions.Open() {
		errnie.Error(position.Validate())
	}

	// Resting venue sells hold the base — Exit would Insufficient funds. Clear
	// them for restored opens before the desk can arm (REST; no private WS yet).
	tier.clearRestingSells()

	if tier.private != nil && host != nil {
		// Dedicated Watch targets — do not share host.Balance() with the WS
		// decode path or REST and websocket frames race on Result.
		// Use(normalizer) is required so REST ledger codes (ZUSD) collapse to
		// desk quote names; without it Hello cash stays 0 while equity still
		// updates from TradeBalance.
		watchBal := kraken.NewBalance().Use(tier.normalizer)
		watchTB := kraken.NewTradeBalance()

		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.Balance,
			watchBal.Request(),
			watchBal,
			func() { tier.ingest(watchBal) },
		)
		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.TradeBalance,
			watchTB.Request(),
			watchTB,
			func() { tier.ingest(watchTB) },
		)
	}

	// Private WS Open + executions subscribe wait for Desk.Start / OpenPrivate
	// so seedBooks can finish without an unread private socket timing out.

	if err := errnie.Require(map[string]any{
		"price":      tier.price,
		"normalizer": tier.price.normalizer,
		"private":    tier.private,
	}); err != nil {
		errnie.Error(err)
		return nil
	}

	// Paper/virtual always arm. Real arms only when this process is live
	// (market.model real); a disarmed real lane in paper boots stays INIT so
	// Desk.Update/Enter cannot mark or spend on it. Skipping READY on live
	// real left restored opens with nil pnl_pct and an empty open-choice chart.
	if tt != TierTypeReal || system.Cfg.Market.Model == "real" {
		tier.Transition(runtime.READY)
	}

	return tier
}

/*
OpenPrivate opens the private websocket (or paper subscribe) after books are
seeded. Boot must not call this during NewTier — an unread private socket
times out while seedBooks waits on Level3.
*/
func (tier *Tier) OpenPrivate() error {
	if tier == nil || tier.private == nil {
		return nil
	}

	if client, ok := tier.private.(*network.WebsocketClient); ok {
		client.Callback(func() error { return tier.subscribeExecutions() })
		return errnie.Error(client.Open())
	}

	if tier.venue != nil {
		return tier.subscribeExecutions()
	}

	return nil
}

/*
clearRestingSells cancels open venue sells for restored open symbols so Exit
can spend the base. Buys are left alone.
*/
func (tier *Tier) clearRestingSells() {
	if tier == nil || tier.private == nil || tier.positions == nil {
		return
	}

	open := tier.positions.Open()
	if len(open) == 0 {
		return
	}

	symbols := make(map[string]struct{}, len(open)*2)

	for _, position := range open {
		if position == nil || position.Symbol == "" {
			continue
		}

		symbols[position.Symbol] = struct{}{}
		if tier.normalizer != nil {
			if name := tier.normalizer.Name(position.Symbol); name != "" {
				symbols[name] = struct{}{}
			}
		}
	}

	orders := NewOrders(tier.private)
	resting := orders.ListOpen()
	if len(resting) == 0 {
		return
	}

	for txid, order := range resting {
		if order.Descr.Type != "sell" {
			continue
		}

		pair := order.Descr.Pair
		if pair == "" {
			continue
		}

		symbol := pair
		if tier.normalizer != nil {
			if name := tier.normalizer.Name(pair); name != "" {
				symbol = name
			}
		}

		if _, ok := symbols[symbol]; !ok {
			if _, ok := symbols[pair]; !ok {
				continue
			}
		}

		if !orders.Cancel(txid) {
			errnie.Error(errnie.Err(
				errnie.IO,
				"[broker.Tier] failed to cancel resting sell "+txid,
				nil,
			))
		}
	}
}

/*
subscribeExecutions opens the private executions channel so add_order acks are
not the only private traffic — fills fold positions without REST lag.
*/
func (tier *Tier) subscribeExecutions() error {
	if tier.private == nil || tier.venue == nil {
		return nil
	}

	req := kraken.NewExecutionSubscription("")

	if tier.Type == TierTypeReal {
		if tier.price == nil || tier.price.Instrument == nil {
			return errnie.Err(
				errnie.Forbidden,
				"[broker.Tier] executions subscribe missing auth",
				nil,
			)
		}

		token, err := tier.price.Instrument.auth.Token(tier.ctx)

		if err != nil {
			return errnie.Error(err)
		}

		req = kraken.NewExecutionSubscription(token)
	}

	buf, err := sonic.Marshal(req)

	if err != nil {
		return errnie.Err(
			errnie.Internal,
			"[broker.Tier] executions subscribe marshal failed",
			err,
		)
	}

	return errnie.Error(tier.private.Write(buf))
}

/*
Hello ...
*/
func (tier *Tier) Hello() map[string]any {
	open := tier.positions.Open()
	positions := make(map[string]*Position, len(open))
	unrealized := decimal.Zero

	for _, position := range open {
		snap := position.SnapshotHello()
		if snap == nil {
			continue
		}

		if snap.PnL != nil {
			unrealized = unrealized.Add(*snap.PnL)
		}

		positions[snap.ID] = snap
	}

	tier.balance.SetUnrealized(&unrealized)

	return map[string]any{
		"positions": positions,
		"balances":  tier.balance.Reading(),
	}
}

/*
Update marks READY positions for the symbol. Desk.Update owns coalesced Hello —
open-universe snapshots are rate-limited, not a per-symbol touch gate.
*/
func (tier *Tier) Update(symbol string, mark *decimal.Decimal) {
	if tier.Status() != runtime.READY || tier.positions == nil {
		return
	}

	position := tier.positions.Get(symbol)

	if position == nil {
		return
	}

	switch position.Status() {
	case runtime.READY, runtime.WAITING:
	default:
		return
	}

	if dirty, err := position.Update(mark); err != nil {
		errnie.Error(err)
	} else if dirty {
		errnie.Error(tier.positions.Save(position))
	}
}

/*
Pending retains flat positions after Remove until strategy records exit_path —
the position is closed in cache but the model still needs the handle for attribution.
*/
func (tier *Tier) Pending() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier.pending.Range(func(_, value any) bool {
			return yield(value.(*Position))
		})
	}
}

/*
ClearPending lets the model acknowledge exit-path attribution after stoploss
settle without deleting the closed record from cache.
*/
func (tier *Tier) ClearPending(symbol string) {
	if symbol == "" {
		return
	}

	tier.pending.Delete(symbol)
}

/*
ProfitableExit is true when fee-net mark PnL% clears PolicyExitMinProfitPct.
Also requires the live bid to clear that floor above the last buy print —
a poisoned Entry (ledger seed / stale VWAP) must not authorize a flat sell.
*/
func (tier *Tier) ProfitableExit(position *Position) bool {
	if tier == nil || tier.price == nil || position == nil {
		return false
	}

	pct := tier.price.PnLPct(position)
	if pct == nil || !clearsExitProfitFloor(pct.InexactFloat64()) {
		return false
	}

	buy := position.lastBuyPrice()
	if buy == nil || buy.Sign() <= 0 {
		return false
	}

	mark := tier.price.Mark(position.Symbol)
	if mark == nil || mark.Sign() <= 0 {
		return false
	}

	move := mark.Sub(*buy).Div(*buy).Mul(decimal.NewFromInt(100))
	return clearsExitProfitFloor(move.InexactFloat64())
}

func clearsExitProfitFloor(pnlPct float64) bool {
	floor := 0.0
	if system.Cfg != nil && system.Cfg.Learning != nil {
		floor = system.Cfg.Learning.PolicyExitMinProfitPct
	}

	if floor <= 0 {
		return pnlPct > 0
	}

	return pnlPct >= floor
}

/*
Enter binds spot wallet to Allocate so mins and cash match what Send will spend.
Cache keys use the normalized symbol so desk mark Hellos hit the same position.
*/
func (tier *Tier) Enter(
	symbol string, path string, pathFound bool, hint SizeHint,
) error {
	if tier.price != nil && tier.price.normalizer != nil {
		symbol = tier.price.normalizer.Name(symbol)
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	if tier.Status() != runtime.READY {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] tier not ready",
			nil,
		)
	}

	qty, err := tier.price.Allocate(symbol, tier.balance, hint)

	if err != nil || qty == nil || qty.Sign() <= 0 {
		if err != nil {
			return errnie.Error(err)
		}

		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] cannot size "+symbol,
			nil,
		)
	}

	spend := tier.enterSpend(symbol, *qty)
	quote := ""
	if pair := tier.price.Instrument.Pair(symbol); pair.Quote != "" {
		quote = pair.Quote
	}
	if spend.Sign() > 0 && quote != "" {
		tier.balance.Reserve(quote, spend)
		tier.reserved.Store(symbol, spend)
	}

	position := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.private,
		tier.settle,
	)
	if err := position.Enter(path, qty, tier.balance); err != nil {
		tier.releaseReserved(symbol, quote)
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, position, true)

	if err := tier.Send(position, "enter"); err != nil {
		// Local / pre-ack failure: no venue order_id yet — Drop so Has(symbol)
		// cannot permanently block re-entry (rejectOrder does the same on ack).
		position.mu.Lock()
		acked := position.EntryVenueOrderID != ""
		position.mu.Unlock()

		if !acked {
			tier.positions.Drop(symbol)
			tier.releaseReserved(symbol, quote)
		}

		return errnie.Error(err)
	}

	return nil
}

func (tier *Tier) enterSpend(symbol string, qty decimal.Decimal) decimal.Decimal {
	if tier.price == nil {
		return decimal.Zero
	}

	filled, gross := tier.price.Walk(symbol, qty, true)
	if filled == nil || gross == nil || filled.Sign() <= 0 {
		return decimal.Zero
	}

	fee := tier.price.Fee(symbol, gross)
	if fee == nil {
		return *gross
	}

	return gross.Add(*fee)
}

func (tier *Tier) releaseReserved(symbol, quote string) {
	if v, ok := tier.reserved.LoadAndDelete(symbol); ok {
		if spend, ok := v.(decimal.Decimal); ok && quote != "" {
			tier.balance.Release(quote, spend)
		}
	}
}

/*
syncReservedAfterBalance frees leftovers after a Balance poll. Buy fills already
CommitSpend; this clears parks for Dropped/canceled WAITING rows and any
symbol that somehow stayed reserved after Qty landed.
*/
func (tier *Tier) syncReservedAfterBalance() {
	quote := ""
	if system.Cfg != nil && system.Cfg.Market != nil {
		quote = system.Cfg.Market.QuoteCurrency
	}

	tier.reserved.Range(func(key, _ any) bool {
		symbol, _ := key.(string)
		if symbol == "" {
			return true
		}

		pos := tier.positions.Get(symbol)
		if pos == nil {
			tier.releaseReserved(symbol, quote)
			return true
		}

		pos.mu.Lock()
		waiting := pos.Qty == nil || pos.Qty.Sign() <= 0
		pos.mu.Unlock()

		if !waiting {
			tier.releaseReserved(symbol, quote)
		}

		return true
	})
}

/*
Exit arms Closing then Send (or fillsExit without transport) so policy exits share
the same sell path as stoploss settle rather than mutating cache timestamps alone.
Pre-send: re-walk the live book — refuse a loss or incomplete liquidation.
*/
func (tier *Tier) Exit(
	position *Position, force bool,
) error {
	if position == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[broker.Tier.Exit] position is nil",
			nil,
		))
	}

	if tier.Status() != runtime.READY {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Exit] tier not ready",
			nil,
		))
	}

	// Stoploss parks BUSY before exit() runs. Skip only when a sell with Volume
	// is already on the wire — Enter used to pre-allocate ExitOrder{ClOrdId}
	// with empty Volume, which deadlocked every live exit here.
	if position.Status() == runtime.BUSY && position.ExitSellArmed() {
		return nil
	}

	if !force && !tier.ProfitableExit(position) {
		tier.abortExit(position)
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Exit] position is not profitable",
			nil,
		))
	}

	// Drop the pre-fill park before Send. A fast fill stores ClosedAt back into
	// pending for creditExit; clearing after Send in Model.Step raced that store.
	tier.pending.Delete(position.Symbol)

	if err := position.exit(force); err != nil {
		return errnie.Error(err)
	}

	if err := tier.Send(position, "exit"); err != nil {
		// Pre-wire failures never reached Kraken — roll BUSY back or the row
		// freezes forever (Update skips BUSY, pending was cleared). Ambiguous
		// Write errors keep Closing so we do not double-sell.
		if sendMissedWire(err) {
			tier.abortExit(position)
		}
		return errnie.Error(err)
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
abortExit clears pending and returns BUSY → READY so the position holds
instead of selling into a loss. AbortPending rolls back unfilled tranche
claims / StoplossTriggered; only a zero-fill abort Disarms.
*/
func (tier *Tier) abortExit(position *Position) {
	if position == nil {
		return
	}

	tier.pending.Delete(position.Symbol)

	position.mu.Lock()

	if position.Stoploss != nil {
		position.Stoploss.AbortPending()
		if position.Stoploss.TranchesExited == 0 {
			position.Stoploss.Disarm()
		}
	}

	position.ExitPath = ""
	position.ExitTrigger = ""
	position.ExitVenueOrderID = ""
	position.ExitOrder = nil

	if position.Status() == runtime.BUSY {
		position.Transition(runtime.READY)
	}

	position.mu.Unlock()

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
	}
}

/*
apply folds new venue fills into the live open for that symbol. Closed cycles
come from Ledger Fold — executions must not replay history onto the open
(snap_trades is off). Flat rows Remove into pending.
*/
func (tier *Tier) apply(execution *market.Execution) {
	if execution == nil {
		return
	}

	for _, data := range execution.Data {
		if data == nil {
			continue
		}

		symbol := data.Symbol
		if tier.normalizer != nil {
			symbol = tier.normalizer.Name(symbol)
		}

		position := tier.positions.Get(symbol)

		if position == nil {
			continue
		}

		if data.ExecType == "canceled" {
			switch position.Status() {
			case runtime.BUSY:
				tier.abortExit(position)
			case runtime.WAITING, runtime.INIT:
				if position.Qty == nil || position.Qty.Sign() <= 0 {
					tier.positions.Drop(position.Symbol)
				}
			}

			continue
		}

		position.Apply(*data)

		if data.Side == "buy" {
			quote := ""
			if tier.price != nil && tier.price.Instrument != nil {
				quote = tier.price.Instrument.Pair(symbol).Quote
			}
			// Commit into local Assets now — do not wait for Balance REST.
			// Outages left reserved parks stuck and Cash collapsed to ~0.
			if v, ok := tier.reserved.LoadAndDelete(symbol); ok {
				if spend, ok := v.(decimal.Decimal); ok {
					tier.balance.CommitSpend(quote, spend)
				}
			}
		}

		if !position.IsClosed() {
			// Partial scale-out: only release BUSY once this ExitOrder is fully
			// filled. Mid-order partials must keep ExitOrder armed or the next
			// tick re-fires a second market sell against the working remainder.
			if data.Side == "sell" &&
				position.Status() == runtime.BUSY &&
				exitOrderFilled(position, data) {
				position.mu.Lock()
				position.ExitOrder = nil
				position.ExitVenueOrderID = ""
				position.ExitTrigger = ""
				if position.Stoploss != nil {
					position.Stoploss.CommitPending()
				}
				position.Transition(runtime.READY)
				position.mu.Unlock()

				if err := tier.positions.Save(position); err != nil {
					errnie.Error(err)
				}
			}
			continue
		}

		if position.Stoploss != nil {
			position.Stoploss.CommitPending()
		}

		tier.positions.Remove(position.Symbol)
		tier.pending.Store(position.Symbol, position)

		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}
}

/*
exitOrderFilled is true when the working ExitOrder has fully matched — by
venue status, CumQty, or summed sell fills on its ClOrdId.
*/
func exitOrderFilled(position *Position, data *market.ExecutionData) bool {
	if position == nil || data == nil {
		return false
	}

	switch data.OrderStatus {
	case "filled", "closed":
		return true
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	if position.ExitOrder == nil || position.ExitOrder.Volume == "" {
		return false
	}

	want, err := decimal.NewFromString(position.ExitOrder.Volume)
	if err != nil || want.Sign() <= 0 {
		return false
	}

	if data.CumQty != nil && data.CumQty.Cmp(want) >= 0 {
		return true
	}

	clOrdID := position.ExitOrder.ClOrdId
	venueID := position.ExitVenueOrderID
	filled := decimal.Zero

	for _, fill := range position.Fills {
		if fill == nil || fill.Side != "sell" || fill.LastQty == nil {
			continue
		}
		switch {
		case clOrdID != "" && fill.ClientOrderID == clOrdID:
		case venueID != "" && fill.OrderID == venueID:
		case clOrdID == "" && venueID == "":
		default:
			continue
		}
		filled = filled.Add(*fill.LastQty)
	}

	return filled.Cmp(want) >= 0
}

/*
settle parks a stoploss-triggered position for model.Exit (sell + attribution).
Open stays until the exit fill flats Qty — Remove belongs to fold after fill.
*/
func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	tier.pending.Store(position.Symbol, position)

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
	}
}

/*
Send writes the enter/exit add_order. Policy exits are gated by exact book PnL
before Send; the wire order stays the market sell on ExitOrder.
*/
func (tier *Tier) Send(position *Position, action string) error {
	position.mu.Lock()

	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	// Copy wire fields under lock — Unlock before Write so fill Apply can proceed.
	var (
		side, orderType, volume, clOrdID string
		symbol                           string
	)

	if order != nil {
		side = order.Type
		orderType = order.OrderType
		volume = order.Volume
		clOrdID = order.ClOrdId
	}

	symbol = position.Symbol
	position.mu.Unlock()

	if order == nil || volume == "" || clOrdID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[broker.Tier.Send] exit/enter order is incomplete",
			nil,
		))
	}

	params := map[string]any{
		"symbol":     symbol,
		"side":       side,
		"order_type": orderType,
		"order_qty":  json.Number(volume),
		"cl_ord_id":  clOrdID,
	}

	if tier.Type == TierTypeReal {
		token, err := tier.price.Instrument.auth.Token(tier.ctx)

		if err != nil {
			return errnie.Error(err)
		}

		params["token"] = token
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "add_order",
		"params": params,
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[broker.Tier.Send] failed to marshal message",
			err,
		))
	}

	return errnie.Error(tier.private.Write(buf))
}

/*
sendMissedWire is true when the order never left the process — safe to roll
BUSY back. Ambiguous Write failures after a possible deliver stay Closing.
*/
func sendMissedWire(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.Contains(msg, "cooling down") ||
		strings.Contains(msg, "not ready for write") ||
		strings.Contains(msg, "order is incomplete") ||
		strings.Contains(msg, "failed to marshal") ||
		strings.Contains(msg, "private REST")
}

/*
ackOrder records venue order_id on the position from the add_order cl_ord_id.
*/
func (tier *Tier) ackOrder(order *kraken.Order) {
	clOrdID := order.Result.ClOrdID
	orderID := order.Result.OrderID

	if clOrdID == "" || orderID == "" {
		return
	}

	for _, position := range tier.positions.Open() {
		if position == nil {
			continue
		}

		position.mu.Lock()

		switch {
		case position.EntryOrder != nil && position.EntryOrder.ClOrdId == clOrdID:
			position.EntryVenueOrderID = orderID
			position.mu.Unlock()
			return

		case position.ExitOrder != nil && position.ExitOrder.ClOrdId == clOrdID:
			position.ExitVenueOrderID = orderID
			position.mu.Unlock()
			return
		}

		position.mu.Unlock()
	}
}

/*
rejectOrder handles failed add_order acks: aborts exit back to READY so the
position holds and can re-arm, or drops an unfilled rejected enter.
*/
func (tier *Tier) rejectOrder(order *kraken.Order) {
	if order == nil {
		return
	}

	errnie.Error(errnie.Err(
		errnie.IO,
		"[broker.Tier.rejectOrder] add_order rejected: "+order.Error,
		nil,
	).With("cl_ord_id", order.Result.ClOrdID))

	clOrdID := order.Result.ClOrdID

	if clOrdID == "" {
		waiting := make([]*Position, 0, 1)
		busy := make([]*Position, 0, 1)

		for _, position := range tier.positions.Open() {
			if position == nil {
				continue
			}

			switch position.Status() {
			case runtime.WAITING:
				position.mu.Lock()
				hasQty := position.Qty != nil && position.Qty.Sign() > 0
				hasVenue := position.EntryVenueOrderID != ""
				position.mu.Unlock()

				if !hasQty && !hasVenue {
					waiting = append(waiting, position)
				}

			case runtime.BUSY:
				position.mu.Lock()
				inFlight := position.ExitOrder != nil &&
					position.ExitVenueOrderID == ""
				position.mu.Unlock()

				if inFlight {
					busy = append(busy, position)
				}
			}
		}

		for _, pos := range busy {
			pos.mu.Lock()
			symbol := pos.Symbol
			vol := ""
			side := ""
			fillQty := ""

			if pos.Qty != nil {
				fillQty = pos.Qty.String()
			}

			if pos.ExitOrder != nil {
				vol = pos.ExitOrder.Volume
				side = pos.ExitOrder.Type
			}

			pos.mu.Unlock()

			errnie.Error(errnie.Err(
				errnie.IO,
				"[broker.Tier.rejectOrder] rejected in-flight order",
				nil,
			).With("symbol", symbol).
				With("side", side).
				With("order_qty", vol).
				With("fill_qty", fillQty).
				With("venue_error", order.Error).
				With("in_flight_exits", len(busy)))
		}

		if len(busy) == 1 {
			tier.abortExit(busy[0])
		}

		if len(waiting) == 1 {
			sym := waiting[0].Symbol
			quote := ""
			if tier.price != nil && tier.price.Instrument != nil {
				quote = tier.price.Instrument.Pair(sym).Quote
			}
			tier.positions.Drop(sym)
			tier.releaseReserved(sym, quote)
		}

		return
	}

	for _, position := range tier.positions.Open() {
		if position == nil {
			continue
		}

		position.mu.Lock()
		isExit := position.ExitOrder != nil && position.ExitOrder.ClOrdId == clOrdID
		isEntry := position.EntryOrder != nil && position.EntryOrder.ClOrdId == clOrdID
		hasQty := position.Qty != nil && position.Qty.Sign() > 0
		symbol := position.Symbol
		position.mu.Unlock()

		if isExit {
			tier.abortExit(position)
			return
		}

		if isEntry && !hasQty {
			quote := ""
			if tier.price != nil && tier.price.Instrument != nil {
				quote = tier.price.Instrument.Pair(symbol).Quote
			}
			tier.positions.Drop(symbol)
			tier.releaseReserved(symbol, quote)
			return
		}
	}
}

/*
Run reads the private socket, demuxes via venue.Read, then ingest acts on the frame.
*/
func (tier *Tier) Run() {
	for {
		select {
		case <-tier.ctx.Done():
			return
		default:
		}

		buf, err := tier.private.Read()

		if err != nil {
			if errnie.Error(tier.ctx.Err()) != nil {
				return
			}

			if !network.Recoverable(err) {
				errnie.Error(errnie.Err(
					errnie.IO, "[tier] private ingress read failed", err,
				))
			}

			continue
		}

		tier.ingest(tier.venue.Read(buf))
	}
}

/*
ingest applies one decoded private frame. venue.Read only demuxes; this owns side effects.
*/
func (tier *Tier) ingest(frame market.MarketType) {
	if frame == nil {
		return
	}

	switch typed := frame.(type) {
	case *kraken.Balance:
		typed.ToMarket(tier.balance)

		if tier.price != nil && tier.price.balance != nil {
			typed.ToMarket(tier.price.balance)
		}

		tier.syncReservedAfterBalance()

		if tier.onBook != nil {
			tier.onBook()
		}

	case *kraken.TradeBalance:
		typed.ToMarket(tier.balance)

		if tier.onBook != nil {
			tier.onBook()
		}

	case *kraken.Execution:
		executions := &market.Execution{}
		typed.ToMarket(executions)
		tier.apply(executions)

	case *kraken.Order:
		if typed.Success {
			tier.ackOrder(typed)
		} else {
			tier.rejectOrder(typed)
		}
	}
}
