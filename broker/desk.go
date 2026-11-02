package broker

import (
	"context"
	"iter"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/kraken"
)

/*
Desk fans work across the virtual, paper, and real tiers.
*/
type Desk struct {
	*runtime.System
	price *Price
	tiers map[TierType]*Tier
}

func NewDesk(
	ctx context.Context,
	transports map[string]Transport,
	price *Price,
) *Desk {
	desk := &Desk{
		System: runtime.NewSystem(ctx, "desk"),
		price:  price,
		tiers:  make(map[TierType]*Tier, 3),
	}

	for _, tt := range []TierType{TierTypeVirtual, TierTypePaper, TierTypeReal} {
		desk.tiers[tt] = NewTier(ctx, tt, transports[tt.String()], price)
	}

	if price != nil {
		price.balance = desk.tiers[TierTypePaper].balance
	}

	desk.Transition(runtime.READY)
	return desk
}

func (desk *Desk) Hello() {
	for _, tier := range desk.tiers {
		tier.Hello()
	}
}

func (desk *Desk) Update(trades []kraken.TradeData) {
	for _, trade := range trades {
		price, err := decimal.NewFromString(trade.Price.String())

		if err != nil {
			continue
		}

		desk.mark(trade.Symbol, &price)
	}
}

func (desk *Desk) Refresh(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" || desk.price == nil {
			continue
		}

		mark := desk.price.CurrentMark(symbol)

		if mark == nil {
			continue
		}

		price, err := decimal.NewFromString(mark.String())

		if err != nil {
			continue
		}

		desk.mark(symbol, &price)
	}
}

func (desk *Desk) mark(symbol string, price *decimal.Decimal) {
	if symbol == "" || price == nil {
		return
	}

	update := market.NewUpdate(symbol, price)

	for _, tier := range desk.tiers {
		tier.Update(update)
	}
}

func (desk *Desk) Apply(executions *kraken.Execution) {
	if executions == nil {
		return
	}

	for _, execution := range executions.Data {
		for _, tier := range desk.tiers {
			for _, position := range tier.positions.Open(execution.Symbol) {
				if position != nil {
					position.Apply(execution)
				}
			}
		}
	}
}

func (desk *Desk) Enter(
	symbol string, path string, tt TierType, pathFound bool,
) error {
	tier := desk.tiers[tt]

	if tier == nil {
		return errnie.Err(
			errnie.NotFound,
			"[desk.Enter] unknown tier",
			nil,
		)
	}

	tier.Unlocked(desk.tiers[TierTypePaper])
	return tier.Enter(symbol, path, pathFound)
}

func (desk *Desk) Exit(position *Position, tt TierType) error {
	if tier := desk.tiers[tt]; tier != nil {
		return tier.Exit(position)
	}

	for _, tier := range desk.tiers {
		if tier.positions.Has(position.Symbol) {
			return tier.Exit(position)
		}
	}

	return errnie.Err(
		errnie.NotFound,
		"[desk.Exit] position not found",
		nil,
	)
}

func (desk *Desk) Has(symbol string) bool {
	for _, tier := range desk.tiers {
		if tier.positions.Has(symbol) {
			return true
		}
	}

	return false
}

func (desk *Desk) Positions(symbol string) []*Position {
	out := make([]*Position, 0, 3)

	for _, tier := range desk.tiers {
		out = append(out, tier.positions.Open(symbol)...)
	}

	return out
}

func (desk *Desk) Position(symbol string) *Position {
	for _, tt := range []TierType{TierTypePaper, TierTypeReal, TierTypeVirtual} {
		if position := desk.PositionOn(symbol, tt); position != nil {
			return position
		}
	}

	return nil
}

func (desk *Desk) PositionOn(symbol string, tt TierType) *Position {
	tier := desk.tiers[tt]

	if tier == nil {
		return nil
	}

	for _, position := range tier.positions.Open(symbol) {
		if position != nil {
			return position
		}
	}

	return nil
}

func (desk *Desk) OpenPositions() []*Position {
	out := make([]*Position, 0)

	for _, tier := range desk.tiers {
		out = append(out, tier.positions.Open()...)
	}

	return out
}

func (desk *Desk) Virtuals() []*Position {
	tier := desk.tiers[TierTypeVirtual]

	if tier == nil {
		return nil
	}

	return tier.positions.All()
}

func (desk *Desk) ClosedVirtuals() []*Position {
	tier := desk.tiers[TierTypeVirtual]

	if tier == nil {
		return nil
	}

	return tier.positions.Closed()
}

/*
Pending yields stoploss-settled lots waiting for the model to record an exit path.
*/
func (desk *Desk) Pending() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		for _, tt := range []TierType{TierTypeVirtual, TierTypePaper, TierTypeReal} {
			tier := desk.tiers[tt]
			if tier == nil {
				continue
			}

			for position := range tier.Pending() {
				if !yield(position) {
					return
				}
			}
		}
	}
}

func (desk *Desk) ClearPending(symbol string) {
	for _, tier := range desk.tiers {
		tier.ClearPending(symbol)
	}
}

func (desk *Desk) CanSize(symbol string) bool {
	if symbol == "" || desk.price == nil {
		return false
	}

	qty, _ := desk.price.Allocate(symbol, nil)
	return qty != nil && qty.Sign() > 0
}

func (desk *Desk) BookStale(symbol string) bool {
	if desk.price == nil || desk.price.Books == nil {
		return true
	}

	return desk.price.Books.StaleSymbol(symbol)
}
