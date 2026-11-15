package broker

import (
	"context"
	"encoding/json"
	"iter"
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
(hub/UI contract). Marks and PnL are owned by Update — Hello only sums and ships.
*/
func (tier *Tier) Hello() {
	open := tier.positions.Open()
	positions := make(map[string]*Position, len(open))
	unrealized := decimal.Zero

	for _, position := range open {
		if position == nil || position.ID == "" {
			continue
		}

		positions[position.ID] = position

		if position.PnL != nil {
			unrealized = unrealized.Add(*position.PnL)
		}
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
Update marks the symbol's open positions, then Hello so the hub always gets a
full positions_snapshot with current PnL / PnLPct — never a sparse delta.
*/
func (tier *Tier) Update(symbol string, mark *decimal.Decimal) {
	touched := false

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
			touched = true
		}
	}

	if touched {
		tier.Hello()
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

	// Seed the hub immediately; the fill Hello that follows replaces this once
	// Apply stamps Qty/Entry/PnL for the chart series.
	tier.Hello()

	return nil
}

/*
Exit arms Closing then Send (or fillsExit without transport) so policy exits share
the same sell path as stoploss settle rather than mutating cache timestamps alone.
Pre-send: re-walk the live book — refuse a loss or incomplete liquidation.
*/
func (tier *Tier) Exit(
	position *Position,
) error {
	if !tier.exitStillProfitable(position) {
		tier.abortExit(position)
		return nil
	}

	if err := position.exit(); err != nil {
		return errnie.Error(err)
	}

	if err := tier.Send(position, "exit"); err != nil {
		// Keep Closing: Write may have delivered the sell. Clearing here
		// allows a second Send while the first fill is in flight.
		return errnie.Error(err)
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.Hello()
	return nil
}

/*
exitStillProfitable re-walks Qty against live bids. Refuses when the book cannot
absorb the full position or fee-net PnL% sits at/below the arm floor (or <= 0).
*/
func (tier *Tier) exitStillProfitable(position *Position) bool {
	if position == nil || tier.price == nil ||
		position.Qty == nil || position.Qty.Sign() <= 0 {
		return false
	}

	vwap, fully := tier.price.Liquidation(position.Symbol, position.Qty)

	if !fully || vwap == nil {
		return false
	}

	position.Mark = vwap
	position.PnL = tier.price.PnL(position)
	position.PnLPct = tier.price.PnLPct(position)

	if position.PnLPct == nil {
		return false
	}

	pnl, _ := position.PnLPct.Float64()

	if pnl <= 0 {
		return false
	}

	arm := tier.price.StoplossArmPctFor(position.Symbol, position.Qty)

	if position.Stoploss != nil && position.Stoploss.ArmPnlPct > arm {
		arm = position.Stoploss.ArmPnlPct
	}

	return pnl >= arm
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

	tier.Hello()
}

/*
apply folds venue fills into matching positions. Desk.Apply fans out here and
must Hello — private ingest Hellos on its own path.
*/
func (tier *Tier) apply(execution *market.Execution) {
	if tier.fold(execution) {
		tier.Hello()
	}
}

/*
fold applies only fills this position owns. Foreign/symbol-only prints must not
flat real money. Flat sells move open → pending after the exchange fill.
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

		for _, data := range execution.Data {
			if data == nil || tier.normalizer.Name(
				data.Symbol,
			) != tier.normalizer.Name(
				position.Symbol,
			) || !position.owns(data) {
				continue
			}

			position.Apply(*data)
			applied = true
		}

		if !applied {
			continue
		}

		touched = true

		if position.ClosedAt != nil {
			tier.pending.Store(position.Symbol, position)
			tier.positions.Remove(position.Symbol)
			continue
		}

		// Apply → READY; stamp mark/pnl before Hello so the chart has a y.
		if position.Status() == runtime.READY {
			mark := tier.price.Mark(position.Symbol)

			if mark == nil {
				mark = position.Entry
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

	tier.Hello()
}

/*
Send is the only wire exit for real orders; token failures drop pending enters
but clear exit cl_ord_id so kickExit can remint after auth heals.
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

	if tier.private == nil || order == nil || volume == "" || clOrdID == "" {
		return nil
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
		tier.Hello()

	case *kraken.TradeBalance:
		typed.ToMarket(tier.balance)
		tier.Hello()

	case *kraken.Execution:
		executions := &market.Execution{}
		typed.ToMarket(executions)

		if tier.fold(executions) {
			tier.Hello()
		}

	case *kraken.Order:
		if typed.Success {
			tier.ackOrder(typed)
		}
	}
}
