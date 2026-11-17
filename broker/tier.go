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
		client.Callback(func() error { return tier.subscribeExecutions() })
		errnie.Error(client.Open())
	} else if host != nil {
		errnie.Error(tier.subscribeExecutions())
	}

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
		if position == nil || len(position.Fills) == 0 || position.ClosedAt != nil {
			continue
		}

		id := position.ID
		pnl := position.PnL

		if id == "" {
			continue
		}

		if pnl != nil {
			unrealized = unrealized.Add(*pnl)
		}

		positions[id] = &Position{
			ID:        position.ID,
			Symbol:    position.Symbol,
			TierKey:   position.TierKey,
			OpenedAt:  position.OpenedAt,
			ClosedAt:  position.ClosedAt,
			EntryPath: position.EntryPath,
			Qty:       position.Qty,
			Entry:     position.Entry,
			Mark:      position.Mark,
			PnL:       position.PnL,
			PnLPct:    position.PnLPct,
		}

		if position.Stoploss != nil {
			positions[id].Stoploss = &Stoploss{
				State:           position.Stoploss.State,
				Regime:          position.Stoploss.Regime,
				InitialPnLPct:   position.Stoploss.InitialPnLPct,
				CurrentPnLPct:   position.Stoploss.CurrentPnLPct,
				PeakPnLPct:      position.Stoploss.PeakPnLPct,
				ArmPnlPct:       position.Stoploss.ArmPnlPct,
				TrailingFloor:   position.Stoploss.TrailingFloor,
				TypicalPullback: position.Stoploss.TypicalPullback,
				Confirms:        position.Stoploss.Confirms,
				ExhaustScore:    position.Stoploss.ExhaustScore,
			}
		}
	}

	tier.balance.Unrealized = &unrealized

	return map[string]any{
		"positions": positions,
		"balances":  tier.balance.Reading(),
	}
}

/*
Update marks READY positions for the symbol. Desk.Update owns Hello — one full
open-universe snapshot per Level3/trade batch, not a per-symbol touch gate.
*/
func (tier *Tier) Update(symbol string, mark *decimal.Decimal) {
	if tier.Status() != runtime.READY {
		return
	}

	for _, position := range tier.positions.Open() {
		if position == nil {
			continue
		}

		switch position.Status() {
		case runtime.READY, runtime.WAITING:
		default:
			continue
		}

		posSymbol := tier.price.normalizer.Name(position.Symbol)

		if posSymbol != symbol {
			continue
		}

		if dirty, err := position.Update(mark); err != nil {
			errnie.Error(err)
		} else if dirty {
			errnie.Error(tier.positions.Save(position))
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

func (tier *Tier) ProfitableExit(position *Position) bool {
	if tier == nil || tier.price == nil || position == nil {
		return false
	}

	pnl := tier.price.PnL(position)

	return pnl != nil && pnl.Sign() > 0
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

	if tier.Status() != runtime.READY {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] tier not ready",
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

	if tier.Status() != runtime.READY {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Exit] tier not ready",
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

	position.mu.Lock()

	if position.Stoploss != nil {
		position.Stoploss.Disarm()
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

		position := tier.positions.Get(data.Symbol)

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

		if position.ClosedAt == nil {
			continue
		}

		tier.positions.Remove(position.Symbol)
		tier.pending.Store(position.Symbol, position)

		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}
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
			tier.positions.Drop(waiting[0].Symbol)
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
