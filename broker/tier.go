package broker

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
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
) *Tier {
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
		ctx, tt.String(), private, price, tier.settle,
	)

	tier.unlocked = tier.Unlocked(nil)

	if host != nil {
		host.Balance().OnChange(func(frame market.MarketType) {
			if tier.Type == TierTypePaper && tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}

			tier.Hello()
		})

		host.Execution().OnChange(func(frame market.MarketType) {
			book := &market.Execution{}
			frame.ToMarket(book)
			tier.apply(book)
		})
	}

	if private != nil && tier.Type == TierTypeReal {
		if err := OpenWS(private, system.Cfg.WebSocket.Endpoints.Private); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO, "[broker.Tier] private websocket open failed", err,
			))
		}
	}

	// Virtual/paper expose private REST themselves. Live WS has no path-only
	// REST signer here — balances arrive on the private socket instead.
	if host != nil && private != nil &&
		(tier.Type == TierTypeVirtual || tier.Type == TierTypePaper) {
		private.Watch(
			system.Cfg.WebSocket.Endpoints.Balance,
			host.Balance().Request(),
			host.Balance(),
		)

		// Prime once so Allocate sees cash before the first Watch tick.
		if raw, err := private.Post(
			system.Cfg.WebSocket.Endpoints.Balance,
			host.Balance().Request(),
		); err == nil {
			_ = json.Unmarshal(raw, host.Balance())
		}
	}

	return tier
}

func (tier *Tier) apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	touched := make(map[string]*Position)

	for _, execution := range executions.Data {
		if execution == nil {
			continue
		}

		matched := false

		for _, position := range tier.positions.Open(execution.Symbol) {
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
		tier.positions.Save(position)
	}

	tier.publishLedger(touched)
}

/*
walletReading projects venue cash plus open mark PnL into the dashboard
{cash, unrealized, equity} shape keyed under balances.<tier>.
*/
func (tier *Tier) walletReading() map[string]float64 {
	quote := system.Cfg.Market.QuoteCurrency
	cash := 0.0
	unrealized := 0.0

	book := market.NewBalance(map[string]any{
		"quote":  quote,
		"assets": []*market.Asset{},
	})

	if tier.venue != nil {
		tier.venue.Balance().ToMarket(book)
	}

	if c := book.Cash(quote); c != nil {
		cash, _ = c.Float64()
	} else if c := book.Cash(book.Quote); c != nil {
		cash, _ = c.Float64()
	} else {
		for _, asset := range book.Assets {
			if asset == nil {
				continue
			}

			if asset.Symbol == quote || asset.Symbol == "Z"+quote {
				cash, _ = asset.Capital.Float64()
				break
			}
		}
	}

	if cash == 0 && tier.Type == TierTypeVirtual {
		cash = float64(system.Cfg.Market.Balance)
	}

	for _, position := range tier.positions.Open() {
		if position == nil || position.PnL == nil {
			continue
		}

		u, _ := position.PnL.Float64()
		unrealized += u
	}

	return map[string]float64{
		"cash":       cash,
		"unrealized": unrealized,
		"equity":     cash + unrealized,
	}
}

/*
publishLedger fans positions and this tier's wallet reading to the hub.
*/
func (tier *Tier) publishLedger(positions map[string]*Position) {
	ui.UI.WriteJSON(map[string]any{
		"positions": positions,
		"balances": map[string]any{
			tier.Type.String(): tier.walletReading(),
		},
	})
}

func (tier *Tier) Hello() {
	positions := make(map[string]*Position)

	for _, position := range tier.positions.All() {
		if position == nil || position.ID == "" {
			continue
		}

		// Closed lots are frozen snapshots — do not re-mark them.
		if position.ClosedAt == nil {
			position.Update(nil)
		}

		positions[position.ID] = position
	}

	tier.publishLedger(positions)
}

func (tier *Tier) Update(data *market.Update) {
	for _, position := range tier.positions.Open(data.Symbol) {
		position.Update(data.Price)

		now := time.Now()

		if position.UpdatedAt != nil && time.Since(*position.UpdatedAt) > 30*time.Second {
			tier.positions.Save(position)
			position.UpdatedAt = &now
		}

		tier.publishLedger(map[string]*Position{position.ID: position})
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

func (tier *Tier) ok() bool {
	return tier.WinRate > 0.5 && tier.Edge > 0
}

/*
Unlocked: virtual/paper always. Real needs paper ok; once real has closed
history it must stay ok too (paper healthy does not keep a failing real open).
*/
func (tier *Tier) Unlocked(paper *Tier) bool {
	if tier.Type != TierTypeReal {
		tier.unlocked = true
		return true
	}

	virgin := tier.WinRate == 0 && tier.Edge == 0
	tier.unlocked = paper != nil && paper.ok() && (virgin || tier.ok())
	return tier.unlocked
}

func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
) error {
	if tier.Type == TierTypeReal && tier.private == nil {
		return errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] real transport not armed",
			nil,
		)
	}

	if !tier.unlocked {
		return errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] tier not unlocked",
			nil,
		)
	}

	// Virtual + resonance bypass explores without the physical desk gates so
	// a lab bypass can actually open sample lots. Paper/real stay gated.
	if !(tier.Type == TierTypeVirtual && system.Cfg.Learning.ResonanceBypass) {
		if decision := CheckEntryPhysical(tier.price, symbol); !decision.Allowed {
			return &EntryDenial{Gate: "physical", Reason: decision.Reason}
		}
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	// Size from this tier's venue wallet (virtual/paper/real all keep one).
	var wallet *market.Balance

	if tier.venue != nil {
		wallet = market.NewBalance(map[string]any{
			"quote":  system.Cfg.Market.QuoteCurrency,
			"assets": []*market.Asset{},
		})
		tier.venue.Balance().ToMarket(wallet)
	}

	qty, err := tier.price.Allocate(symbol, wallet)

	if err != nil || qty == nil || qty.Sign() <= 0 {
		reason := "cannot size " + symbol
		if err != nil {
			reason = err.Error()
		}

		return &EntryDenial{Gate: "size", Reason: reason}
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
		if _, ok := err.(*EntryDenial); ok {
			return err
		}

		return errnie.Error(err)
	}

	tier.positions.Add(symbol, pos, true)

	if err := tier.Send(pos, "enter"); err != nil {
		// Send already Drop'd the lot. Soft cash/base denials are routine.
		if errnie.IsNotAcceptable(err) {
			return &EntryDenial{Gate: "size", Reason: err.Error()}
		}

		return errnie.Error(err)
	}

	// Fill Apply (async on Run) owns the first Save/publish with fills+pnl.
	// No-transport lots seed in Enter — persist and push them now.
	if pos.transport == nil {
		pos.Update(nil)
		tier.positions.Save(pos)
		tier.publishLedger(map[string]*Position{pos.ID: pos})
	}

	return nil
}

func (tier *Tier) Exit(
	position *Position,
) error {
	if err := tier.Send(position, "exit"); err != nil {
		return errnie.Error(err)
	}

	tier.positions.Remove(position.Symbol)
	tier.publishLedger(map[string]*Position{position.ID: position})

	tier.pending.Delete(position.Symbol)
	return nil
}

func (tier *Tier) Send(position *Position, action string) error {
	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	if tier.private != nil && order != nil {
		msg := map[string]any{
			"method": "add_order",
			"params": map[string]any{
				"symbol":     position.Symbol,
				"side":       order.Type,
				"order_type": order.OrderType,
				"order_qty":  order.Volume,
				"cl_ord_id":  order.ClOrdId,
			},
		}

		buf, err := sonic.Marshal(msg)

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
			if action == "enter" {
				tier.positions.Drop(position.Symbol)
			}

			if errnie.IsNotAcceptable(err) {
				return err
			}

			return errnie.Error(err)
		}
	}

	return nil
}

func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	if err := tier.Send(position, "exit"); err != nil {
		errnie.Error(err)
		return
	}

	tier.positions.Remove(position.Symbol)
	tier.Recalculate()
	tier.pending.Store(position.Symbol, position)
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

	if tier.Type == TierTypeReal && !tier.ok() {
		tier.unlocked = false
	}
}

/*
Run owns the private ingress for this tier. Public frames are read once on Desk.
*/
func (tier *Tier) Run() {
	if tier.private == nil || tier.venue == nil {
		return
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
