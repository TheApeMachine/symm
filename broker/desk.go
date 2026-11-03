package broker

import (
	"context"
	"iter"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/venue/kraken"
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
	transports map[string]network.Client,
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
		if symbol == "" {
			continue
		}

		// Read the bid under Book's RLock, then mark outside — mark→Update
		// re-enters Book and recursive RLock deadlocks against L3 writers.
		var price *decimal.Decimal

		desk.price.Books.Book(symbol, func(managed *market.Book) {
			if managed == nil {
				return
			}

			bid := managed.BestBid()
			if bid == nil || bid.Price == nil {
				return
			}

			price = bid.Price
		})

		if price != nil {
			desk.mark(symbol, price)
		}
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
			matched := false

			for _, position := range tier.positions.Open(execution.Symbol) {
				if position == nil || !position.Owns(execution) {
					continue
				}

				position.Apply(execution)
				matched = true
			}

			// Exit fills can land after the lot has already moved to closed.
			// TODO: That should not happen, and this is the whole reason you
			// need to learn god damn encapsulation for once.
			if matched {
				continue
			}

			for _, position := range tier.positions.Closed(execution.Symbol) {
				if position == nil || !position.Owns(execution) {
					continue
				}

				position.Apply(execution)
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

func (desk *Desk) Exit(
	tier TierType, position *Position,
) error {
	return desk.tiers[tier].Exit(position)
}

func (desk *Desk) Has(symbol string) bool {
	for _, tier := range desk.tiers {
		if tier.positions.Has(symbol) {
			return true
		}
	}

	return false
}

func (desk *Desk) Positions(tier TierType, symbol string) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tier]

		if tier == nil {
			return
		}

		for _, position := range tier.positions.Open(symbol) {
			if !yield(position) {
				return
			}
		}
	}
}

/*
Pending yields stoploss-settled lots waiting for the model to record an exit path.
*/
func (desk *Desk) Pending(tier TierType) iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier := desk.tiers[tier]

		if tier == nil {
			return
		}

		for position := range tier.Pending() {
			if !yield(position) {
				return
			}
		}
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
