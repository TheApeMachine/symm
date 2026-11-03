package broker

import (
	"context"
	"iter"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
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
	transport network.Client
	price     *Price
	balance   *market.Balance
	positions *Cache
	pending   *sync.Map
	unlocked  bool
}

func NewTier(
	ctx context.Context,
	tt TierType,
	transport network.Client,
	price *Price,
) *Tier {
	balance := market.NewBalance(map[string]any{
		"quote": system.Cfg.Market.QuoteCurrency,
		"assets": []*market.Asset{
			market.NewAsset(
				nil,
				system.Cfg.Market.QuoteCurrency,
				decimal.NewFromInt(int64(system.Cfg.Market.Balance)),
				decimal.Zero,
				decimal.Zero,
			),
		},
	})

	tier := &Tier{
		ctx:       ctx,
		Type:      tt,
		transport: transport,
		price:     price,
		balance:   balance,
		pending:   &sync.Map{},
	}

	tier.positions = NewCache(
		ctx, tt.String(), transport, price, tier.settle,
	)

	tier.unlocked = tier.Unlocked(nil)

	return tier
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

	ui.UI.WriteJSON(map[string]any{
		tier.Type.String(): map[string]any{
			"positions": positions,
			"balance":   tier.balance,
		},
	})
}

func (tier *Tier) Update(data *market.Update) {
	var mark *decimal.Decimal

	if data.Price != nil {
		parsed, _ := decimal.NewFromString(data.Price.String())
		mark = &parsed
	}

	for _, position := range tier.positions.Open(data.Symbol) {
		position.Update(mark)
		ui.UI.WriteJSON(map[string]any{
			tier.Type.String(): map[string]any{
				"positions": map[string]any{position.ID: position},
			},
		})
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
	if tier.Type == TierTypeReal && tier.transport == nil {
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

	// Virtual learning lots skip venue qty/cost mins (nil wallet). Paper and
	// real pass the tier balance so Allocate enforces QtyMin/CostMin.
	var wallet *market.Balance

	if tier.Type != TierTypeVirtual {
		wallet = tier.balance
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
		tier.transport,
		tier.settle,
	)

	if err := pos.Enter(path, qty, wallet); err != nil {
		if _, ok := err.(*EntryDenial); ok {
			return err
		}

		return errnie.Error(err)
	}

	// 2. Add to active cache
	tier.positions.Add(symbol, pos, true)
	pos.Update(nil)

	ui.UI.WriteJSON(map[string]any{
		tier.Type.String(): map[string]any{
			"positions": map[string]any{pos.ID: pos},
		},
	})

	if err := tier.Send(pos, "enter"); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (tier *Tier) Exit(
	position *Position,
) error {
	tier.positions.Remove(position.Symbol)

	ui.UI.WriteJSON(map[string]any{
		tier.Type.String(): map[string]any{
			"positions": map[string]any{position.ID: position},
		},
	})

	if err := tier.Send(position, "exit"); err != nil {
		return errnie.Error(err)
	}

	tier.pending.Delete(position.Symbol)
	return nil
}

func (tier *Tier) Send(position *Position, action string) error {
	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	if tier.transport != nil && order != nil {
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
			tier.positions.Drop(position.Symbol)

			return errnie.Err(
				errnie.Internal,
				"[broker.Tier.Enter] failed to marshal message",
				err,
			)
		}

		if err := tier.transport.Write(buf); err != nil {
			tier.positions.Drop(position.Symbol)
			return errnie.Error(err)
		}
	}

	return nil
}

func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	// Paper/real must place the venue sell; virtual has no transport.
	if err := tier.Send(position, "exit"); err != nil {
		errnie.Error(err)
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
