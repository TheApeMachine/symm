package broker

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

type TierType uint8

const (
	TierTypeVirtual TierType = iota
	TierTypePaper
	TierTypeReal
)

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

type Tier struct {
	ctx       context.Context
	Type      TierType
	WinRate   float64
	Edge      float64
	public    network.Client
	private   network.Client
	price     *Price
	venue     market.Venue
	positions *Cache
	pending   *sync.Map
	unlocked  bool
}

func NewTier(
	ctx context.Context,
	tt TierType,
	public network.Client,
	private network.Client,
	price *Price,
	host market.Venue,
	client *store.Client,
) *Tier {
	// Real orders leave the process only when market.model is real.
	if tt == TierTypeReal && system.Cfg.Market.Model != "real" {
		private = nil
	}

	tier := &Tier{
		ctx:     ctx,
		Type:    tt,
		public:  public,
		private: private,
		price:   price,
		venue:   host,
		pending: &sync.Map{},
	}

	tier.positions = NewCache(
		ctx, tt.String(), private, price, tier.settle, client,
	)

	if host != nil {
		host.Balance().OnChange(func(frame market.MarketType) {
			// BookIntact / CanSize size through price.balance; keep it
			// on the same spot ledger Enter spends (not paper-only).
			if tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}

			tier.Hello()
		})

		host.TradeBalance().OnChange(func(market.MarketType) {
			tier.Hello()
		})

		host.Execution().OnChange(func(frame market.MarketType) {
			execution := &market.Execution{}
			frame.ToMarket(execution)
			tier.apply(execution)
		})

		if book, ok := host.(*venue.Kraken); ok && book.Order() != nil {
			book.Order().OnChange(func(frame market.MarketType) {
				order, ok := frame.(*kraken.Order)

				if !ok || order == nil || order.Success {
					return
				}

				tier.rejectOrder(order)
			})
		}
	}

	if tier.private != nil && host != nil {
		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.Balance,
			host.Balance().Request(),
			host.Balance(),
		)

		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.TradeBalance,
			host.TradeBalance().Request(),
			host.TradeBalance(),
		)
	}

	tier.Recalculate()
	tier.unlocked = tier.Unlocked(nil)

	return tier
}

func (tier *Tier) apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	touched := make(map[string]*Position)

	for _, execution := range executions.Data {
		if execution == nil || !tier.fillReport(execution) {
			continue
		}

		matched := false

		for _, position := range tier.positions.Submitted(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
			matched = true
		}

		if matched {
			continue
		}

		for _, position := range tier.positions.Closed(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
		}
	}

	if len(touched) == 0 {
		return
	}

	for _, position := range touched {
		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}

	tier.publishLedger(touched)
}

/*
publishLedger fans positions and this tier's balance to the hub.
*/
func (tier *Tier) publishLedger(positions map[string]*Position) {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	if positions == nil {
		positions = map[string]*Position{}
	}

	balances := map[string]any{}

	if tier.venue != nil {
		balances[tier.Type.String()] = tier.venue.Balance()
	}

	ui.UI.WriteJSON(map[string]any{
		"positions": positions,
		"balances":  balances,
	})
}

/*
fillReport keeps trade prints. Status-only execution events are not fills.
*/
func (tier *Tier) fillReport(execution *market.ExecutionData) bool {
	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return false
	}

	return execution.ExecType == "" || execution.ExecType == "trade"
}

func (tier *Tier) Hello() {
	positions := make(map[string]*Position)

	for _, position := range tier.positions.All() {
		if position == nil || position.ID == "" {
			continue
		}

		// Closed lots are frozen snapshots — do not re-mark them.
		if position.ClosedAt == nil {
			mark := position.Mark

			if mark == nil {
				if tier.price == nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[tier.Hello] price is not wired",
						nil,
					), "symbol", position.Symbol)
					continue
				}

				mark = tier.price.TouchBid(position.Symbol)

				if mark == nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						"[tier.Hello] no touch bid for "+position.Symbol,
						nil,
					))
					continue
				}
			}

			if err := position.Update(mark); err != nil {
				errnie.Error(err)
			}
		}

		positions[position.ID] = position
	}

	tier.publishLedger(positions)
}

func (tier *Tier) Update(data *market.Update) {
	touched := make(map[string]*Position)

	for _, position := range tier.positions.Open(data.Symbol) {
		if err := position.Update(data.Price); err != nil {
			errnie.Error(err)
			continue
		}

		tier.positions.SaveAsync(position)
		touched[position.ID] = position
	}

	if len(touched) > 0 {
		tier.publishLedger(touched)
	}
}

func (tier *Tier) Pending() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier.pending.Range(func(_, value any) bool {
			return yield(value.(*Position))
		})
	}
}

func (tier *Tier) ClearPending(symbol string) {
	if symbol == "" {
		return
	}

	tier.pending.Delete(symbol)
}

func (tier *Tier) virgin() bool {
	return tier.WinRate == 0 && tier.Edge == 0
}

func (tier *Tier) ok() bool {
	return tier.WinRate > 0.5 && tier.Edge > 0
}

/*
Unlocked: virtual and paper always. Live-only (market.model real) skips the
paper proof — entries are gated upstream and exit on stoploss. Otherwise real
needs paper win-rate above 50% and edge above 0. Once real has history it must
stay there too; after it slips, paper has to lose that proof and earn it again
before real entries resume.
*/
func (tier *Tier) Unlocked(paper *Tier) bool {
	if tier.Type != TierTypeReal {
		tier.unlocked = true
		return true
	}

	if system.Cfg.Market.Model == "real" {
		tier.unlocked = true
		return true
	}

	paperOK := paper != nil && paper.ok()

	if !paperOK {
		tier.unlocked = false
		return false
	}

	if !tier.unlocked {
		if !tier.virgin() && !tier.ok() {
			return tier.unlocked
		}

		tier.unlocked = true
		return true
	}

	if !tier.virgin() && !tier.ok() {
		tier.unlocked = false
		return false
	}

	tier.unlocked = true
	return true
}

func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
) error {
	if !tier.unlocked {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] tier not unlocked", nil,
		))
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	wallet := tier.spotBalance()
	qty, err := tier.price.Allocate(symbol, wallet)

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

	pos := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.private,
		tier.settle,
	)

	if err := pos.Enter(path, qty, wallet); err != nil {
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, pos, true)

	if err := tier.Send(pos, "enter"); err != nil {
		return errnie.Error(err)
	}

	// Fill Apply owns the first Save/publish. No-transport desks Apply a
	// book fill here so Enter never seeds outside Apply.
	if pos.transport == nil {
		if err := tier.fillsEnter(pos); err != nil {
			tier.positions.Drop(symbol)
			return err
		}
	}

	return nil
}

func (tier *Tier) fillsEnter(position *Position) error {
	if position == nil || position.EntryOrder == nil {
		return nil
	}

	qty, err := decimal.NewFromString(position.EntryOrder.Volume)
	if err != nil || qty.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] bad entry volume",
			err,
		)
	}

	px := tier.price.FillAsk(position.Symbol, &qty)
	if px == nil || px.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] no book price for "+position.Symbol,
			nil,
		)
	}

	now := time.Now().UTC()
	position.Apply(market.ExecutionData{
		ExecID:        "local-" + position.ID + "-enter",
		ExecType:      "trade",
		ClientOrderID: position.EntryOrder.ClOrdId,
		Symbol:        position.Symbol,
		Side:          "buy",
		LastQty:       &qty,
		LastPrice:     px,
		AvgPrice:      px,
		CumQty:        &qty,
		OrderStatus:   "filled",
		Timestamp:     now,
	})

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

func (tier *Tier) Exit(
	position *Position,
) error {
	// Policy path arms Closing; exit fills in Apply stamp ClosedAt, then
	// settle Removes. Do not archive before the sell prints.
	if err := position.Exit(); err != nil {
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

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

/*
spotBalance copies the venue ledger into a market.Balance for Allocate/Enter.
*/
func (tier *Tier) spotBalance() *market.Balance {
	quote := system.Cfg.Market.QuoteCurrency
	wallet := market.NewBalance(map[string]any{
		"quote":  quote,
		"assets": []*market.Asset{},
	})

	if tier.venue != nil {
		tier.venue.Balance().ToMarket(wallet)
	}

	return wallet
}

func (tier *Tier) Send(position *Position, action string) error {
	if action == "exit" {
		position.mu.Lock()
		position.ensureExitOrder()
		position.mu.Unlock()
	}

	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	if tier.private == nil || order == nil {
		return nil
	}

	params := map[string]any{
		"symbol":     position.Symbol,
		"side":       order.Type,
		"order_type": order.OrderType,
		"order_qty":  json.Number(order.Volume),
		"cl_ord_id":  order.ClOrdId,
	}

	if tier.Type == TierTypeReal && tier.price != nil && tier.price.Instrument != nil {
		token, err := tier.price.Instrument.WebSocketToken(tier.ctx)

		if err != nil {
			if action == "enter" {
				tier.positions.Drop(position.Symbol)
			}

			return errnie.Error(err)
		}

		params["token"] = token
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "add_order",
		"params": params,
	})

	if err != nil {
		if action == "enter" {
			tier.positions.Drop(position.Symbol)
		}

		return errnie.Err(
			errnie.Internal,
			"[broker.Tier.Send] failed to marshal message",
			err,
		)
	}

	if err := tier.private.Write(buf); err != nil {
		// Keep the lot: Write may have delivered. Fills still Apply by cl_ord_id.
		return errnie.Error(err)
	}

	return nil
}

/*
rejectOrder matches a failed add_order to the lot by cl_ord_id on Entry/ExitOrder.
*/
func (tier *Tier) rejectOrder(order *kraken.Order) {
	clOrdID := order.Result.ClOrdID

	if clOrdID == "" {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[broker.tier] order rejected without cl_ord_id", nil,
		).With("reason", order.Error))
		return
	}

	for _, position := range tier.positions.Submitted() {
		if position == nil {
			continue
		}

		position.mu.Lock()

		switch {
		case position.EntryOrder != nil && position.EntryOrder.ClOrdId == clOrdID:
			filled := position.Qty != nil && position.Qty.Sign() > 0
			position.mu.Unlock()

			if filled {
				errnie.Error(errnie.Err(
					errnie.NotAcceptable, "[broker.tier] entry rejected after fill", nil,
				).With("symbol", position.Symbol, "reason", order.Error))
				tier.Hello()
				return
			}

			tier.positions.Drop(position.Symbol)
			errnie.Error(errnie.Err(
				errnie.NotAcceptable, "[broker.Tier] entry rejected", nil,
			).With("symbol", position.Symbol, "reason", order.Error))
			tier.Hello()
			return

		case position.ExitOrder != nil && position.ExitOrder.ClOrdId == clOrdID:
			position.ExitOrder.ClOrdId = ""
			position.mu.Unlock()

			errnie.Error(errnie.Err(
				errnie.NotAcceptable, "[broker.tier] exit rejected", nil,
			).With("symbol", position.Symbol, "reason", order.Error))
			return
		}

		position.mu.Unlock()
	}
}

func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	// Exit fills landed — archive and hand to the model.
	if position.ClosedAt != nil {
		tier.positions.Remove(position.Symbol)
		tier.Recalculate()
		tier.pending.Store(position.Symbol, position)
		tier.publishLedger(map[string]*Position{position.ID: position})
		return
	}

	// Closing armed — send the exit and stay open until Apply flats the lot.
	if err := tier.Send(position, "exit"); err != nil {
		errnie.Error(err)
		return
	}

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
		return
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
}

func (tier *Tier) Recalculate() {
	closed := tier.positions.Closed()

	if len(closed) == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	wins := 0
	totalReturn := 0.0
	count := 0

	for _, pos := range closed {
		if pos == nil || pos.ClosedAt == nil || pos.PnLPct == nil {
			continue
		}

		pnlPct, _ := pos.PnLPct.Float64()
		totalReturn += pnlPct

		if pnlPct > 0 {
			wins++
		}

		count++
	}

	if count == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	tier.WinRate = float64(wins) / float64(count)
	tier.Edge = totalReturn / float64(count)
}

/*
Run owns the private ingress for this tier. Public frames are read once on Desk.
*/
func (tier *Tier) Run() {
	if tier.private == nil || tier.venue == nil {
		return
	}

	// Resend exits for lots restored mid-close once private ingress is up.
	for _, position := range tier.positions.Open() {
		if position != nil && position.Closing && position.ClosedAt == nil {
			tier.settle(position)
		}
	}

	for {
		select {
		case <-tier.ctx.Done():
			return
		default:
		}

		buf, err := tier.private.Read()

		if err != nil {
			if tier.ctx.Err() != nil {
				return
			}

			continue
		}

		tier.venue.Read(buf)
	}
}
