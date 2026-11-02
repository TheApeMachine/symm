package broker

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	symmdecimal "github.com/theapemachine/symm/decimal"
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
	transport Transport
	price     *Price
	balance   *market.Balance
	positions *Cache
	pending   *sync.Map
	unlocked  bool
}

func NewTier(
	ctx context.Context,
	tt TierType,
	transport Transport,
	price *Price,
) *Tier {
	balance := market.NewBalance(system.Cfg.Market.QuoteCurrency)
	balance.Seed(decimal.NewFromInt(int64(system.Cfg.Market.Balance)))

	switch feed := transport.(type) {
	case *Paper:
		balance.Watch(ctx, time.Second/time.Duration(system.Cfg.WebSocket.CallsPerSecond), feed)
	default:
		if transport != nil {
			balance.Watch(ctx, time.Second/time.Duration(system.Cfg.WebSocket.CallsPerUpdate), NewLiveBalances())
		}
	}

	tier := &Tier{
		ctx:       ctx,
		Type:      tt,
		transport: transport,
		price:     price,
		balance:   balance,
		positions: NewCache(ctx, tt.String(), transport),
		pending:   &sync.Map{},
	}

	tier.unlocked = tier.Unlocked(nil)

	balance.OnChange(func(book *market.Balance) {
		if ui.UI == nil {
			return
		}

		reading := book.Reading()

		if reading == nil {
			return
		}

		ui.UI.WriteJSON(map[string]any{
			"balances": map[string]any{
				tier.Type.String(): reading,
			},
		})
	})

	return tier
}

func (tier *Tier) Hello() {
	ui.UI.WriteJSON(map[string]any{
		"balances": map[string]any{
			tier.Type.String(): tier.balance.Reading(),
		},
		"positions": tier.positions.All(),
	})
}

func (tier *Tier) Update(data *market.Update) {
	var mark *symmdecimal.Decimal

	if data.Price != nil {
		mark, _ = symmdecimal.NewFromString(data.Price.String())
	}

	for _, position := range tier.positions.All(data.Symbol) {
		position.Update(mark)
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
	if !tier.unlocked {
		return errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] tier not unlocked",
			nil,
		)
	}

	if decision := CheckEntryPhysical(tier.price, symbol); !decision.Allowed {
		return &EntryDenial{Gate: "physical", Reason: decision.Reason}
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	pos := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.transport,
		func(position *Position) {
			tier.positions.Remove(position.Symbol)
			tier.Recalculate()
			tier.pending.Store(position.Symbol, position)
		},
	)

	// Real sizes from its own wallet. Paper and virtual keep the desk balance.
	var wallet *market.Balance

	if tier.Type == TierTypeReal {
		wallet = tier.balance
	}

	if err := pos.Enter(path, nil, wallet); err != nil {
		return errnie.Error(err)
	}

	// 2. Add to active cache
	tier.positions.Add(symbol, pos, true)

	if err := tier.Send(pos, "enter"); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (tier *Tier) Exit(
	position *Position,
) error {
	tier.positions.Remove(position.Symbol)

	if err := tier.Send(position, "exit"); err != nil {
		return errnie.Error(err)
	}

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
