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
	"github.com/theapemachine/symm/ui"
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

	if orders := NewOrders(private); orders != nil {
		orders.Restore(append(tier.positions.Open(), tier.positions.Closed()...))
	}

	for _, position := range tier.positions.Open() {
		errnie.Error(position.Validate())
	}

	if tier.private != nil && host != nil {
		balance := host.Balance()
		tradeBalance := host.TradeBalance()

		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.Balance,
			balance.Request(),
			balance,
			func() { tier.ingest(balance) },
		)
		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.TradeBalance,
			tradeBalance.Request(),
			tradeBalance,
			func() { tier.ingest(tradeBalance) },
		)
	}

	if client, ok := tier.private.(*network.WebsocketClient); ok {
		errnie.Error(client.Open())
	}

	if err := errnie.Require(map[string]any{
		"price":      tier.price,
		"normalizer": tier.price.normalizer,
		"private":    tier.private,
	}); err != nil {
		errnie.Error(err)
		return nil
	}

	return tier
}

/*
Hello publishes a full positions_snapshot of open positions as an id-keyed map
(hub/UI contract). Desk.Update is the only live caller — Level3 consolidates the
open book. READY, WAITING, and BUSY all ship — BUSY is still open on the venue
until fold removes it after the exit fill. Omitting BUSY made the chart drop
rows that the exchange still held.
*/
func (tier *Tier) Hello() {
	open := tier.positions.Open()
	positions := make(map[string]*Position, len(open))
	unrealized := decimal.Zero

	for _, position := range open {
		if position == nil {
			continue
		}

		id, pnl := position.snapshotPnL()

		if id == "" {
			continue
		}

		if pnl != nil {
			unrealized = unrealized.Add(*pnl)
		}

		positions[id] = position
	}

	tier.balance.Unrealized = &unrealized
	reading := tier.balance.Reading()

	ui.UI.WriteJSON(map[string]any{
		"tier":      tier.Type.String(),
		"positions": positions,
		"balances": map[string]any{
			tier.Type.String(): reading,
		},
		"positions_snapshot": true,
	})
}

/*
Update marks READY positions for the symbol. Desk.Update owns Hello — one full
open-universe snapshot per Level3/trade batch, not a per-symbol touch gate.
*/
func (tier *Tier) Update(symbol string, mark *decimal.Decimal) {
	for _, position := range tier.positions.Open() {
		if position == nil || position.Status() != runtime.READY {
			continue
		}

		posSymbol := tier.price.normalizer.Name(position.Symbol)

		if posSymbol != symbol {
			continue
		}

		if err := position.Update(mark); err == nil {
			tier.positions.Save(position)
		}
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
Enter binds spot wallet to Allocate so mins and cash match what Send will spend.
Cache keys use the normalized symbol so desk mark Hellos hit the same position.
*/
func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
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

	if tier.price != nil && !tier.price.EntryDragOK(symbol) {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] open drag exceeds max_entry_open_drag_pct",
			nil,
		)
	}

	qty, err := tier.price.Allocate(symbol, tier.balance)

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
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, position, true)

	if err := tier.Send(position, "enter"); err != nil {
		return errnie.Error(err)
	}

	// Chart consolidates on the next Level3 desk.Update Hello — no seed here.
	return nil
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

	if !force {
		if pnl := tier.price.PnL(position); pnl == nil || pnl.Sign() <= 0 {
			tier.abortExit(position)
			return errnie.Error(errnie.Err(
				errnie.Forbidden,
				"[broker.Tier.Exit] position is not profitable",
				nil,
			))
		}
	}

	err := position.exit(force)

	if err != nil {
		return errnie.Error(err)
	}

	if err := tier.Send(position, "exit"); err != nil {
		// Keep Closing unless Send itself aborted: Write may have delivered the
		// sell. Clearing on uncertain send allows a double sell in flight.
		return errnie.Error(err)
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
abortExit clears pending, resets Stoploss, and returns BUSY → READY so the
position holds instead of selling into a loss.
*/
func (tier *Tier) abortExit(position *Position) {
	if position == nil {
		return
	}

	tier.pending.Delete(position.Symbol)
	position.AbortExit()

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
	}
}

/*
apply folds venue fills into matching positions. Desk.Apply fans out here;
the next Level3 desk.Update Hello consolidates the open book.
*/
func (tier *Tier) apply(execution *market.Execution) {
	tier.fold(execution)
}

/*
fold applies only fills this position owns. Foreign/symbol-only prints must not
flat real money. Flat sells move open → pending after the exchange fill;
canceled/expired IOC exits with remaining Qty return BUSY → READY.
*/
func (tier *Tier) fold(execution *market.Execution) bool {
	if execution == nil {
		return false
	}

	touched := false

	for _, position := range tier.positions.All() {
		if position == nil {
			continue
		}

		applied := false
		abortedOrDropped := false

		for _, data := range execution.Data {
			if data == nil || tier.normalizer.Name(
				data.Symbol,
			) != tier.normalizer.Name(
				position.Symbol,
			) || !position.owns(data) {
				continue
			}

			execType := strings.ToLower(data.ExecType)
			orderStatus := strings.ToLower(data.OrderStatus)
			isCancel := execType == "canceled" || execType == "expired" ||
				orderStatus == "canceled" || orderStatus == "expired"

			if isCancel {
				if position.isExitOrder(data) && position.remainingQty().Sign() > 0 {
					tier.abortExit(position)
					touched = true
					abortedOrDropped = true
					break
				}

				if position.isEntryOrder(data) && position.remainingQty().Sign() <= 0 {
					tier.positions.Drop(position.Symbol)
					touched = true
					abortedOrDropped = true
					break
				}

				continue
			}

			if (execType == "" || execType == "trade") &&
				data.LastQty != nil && data.LastQty.Sign() > 0 {
				position.Apply(*data)
				applied = true
			}
		}

		if abortedOrDropped {
			if !applied {
				continue
			}
		}

		if !applied {
			continue
		}

		touched = true

		position.mu.Lock()
		closed := position.ClosedAt != nil
		position.mu.Unlock()

		if closed {
			tier.pending.Store(position.Symbol, position)
			tier.positions.Remove(position.Symbol)
			continue
		}

		// Apply → READY; stamp mark/pnl before Hello so the chart has a y.
		if position.Status() == runtime.READY {
			mark := tier.price.Mark(position.Symbol)

			if mark == nil {
				position.mu.Lock()
				mark = position.Entry
				position.mu.Unlock()
			}

			if mark != nil {
				errnie.Error(position.Update(mark))
			}
		}

		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}

	return touched
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
ackOrder records venue order_id on the position so fills without cl_ord_id still Owns.
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
			tier.positions.Drop(symbol)
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

		tier.positions.Bound(tier.balance)

	case *kraken.TradeBalance:
		typed.ToMarket(tier.balance)

	case *kraken.Execution:
		executions := &market.Execution{}
		typed.ToMarket(executions)
		tier.fold(executions)

	case *kraken.Order:
		if typed.Success {
			tier.ackOrder(typed)
		} else {
			tier.rejectOrder(typed)
		}
	}
}
