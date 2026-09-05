package strategy

import (
	"math/big"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
LearningAction identifies an intervention and its binary quantity refinement.
Power zero uses available executable quantity; successive powers bisect that
range down to venue minimums. The learner chooses the refinement.
*/
type LearningAction struct {
	Kind   types.Action `json:"kind"`
	Power  uint16       `json:"power"`
	Reduce bool         `json:"reduce"`
}

/*
virtualWallet owns one independent, unlevered account. Exact rational arithmetic
preserves the supplied decimals across mixed price and quantity precisions.
It models taker IOC execution on later displayed depth, with account fees;
hidden liquidity, maker fills and counterfactual market impact are not modeled.
*/
type virtualWallet struct {
	cash, quantity, fees                   big.Rat
	fee, factor, lot, minimum, costMinimum big.Rat
	pair                                   kraken.InstrumentPair
	scale                                  int
}

/* initialize fixes the account's starting capital and supplied venue rules. */
func (wallet *virtualWallet) initialize(initial *decimal.Decimal, pair kraken.InstrumentPair, fee *decimal.Decimal) {
	wallet.pair = pair
	wallet.cash.Set(initial.Rat())
	wallet.lot.Set(pair.QtyIncrement.Rat())
	wallet.minimum.Set(pair.QtyMin.Rat())
	wallet.costMinimum.Set(pair.CostMin.Rat())
	// Kraken reports fee percentages; division by 100 converts to a fraction.
	wallet.fee.Quo(fee.Rat(), big.NewRat(100, 1))
	wallet.factor.Add(big.NewRat(1, 1), &wallet.fee)
	// A decimal product needs the sum of its operand scales. Percent conversion
	// adds two decimal places. This bounds exact journal formatting, not math.
	wallet.scale = int(max(initial.GetScale(), int64(pair.PricePrecision+pair.QtyPrecision)+fee.GetScale()+2))
}

/* floor rounds down in exact venue lots, allowing output to alias quantity. */
func (wallet *virtualWallet) floor(output, quantity *big.Rat) *big.Rat {
	output.Quo(quantity, &wallet.lot)
	output.Num().Quo(output.Num(), output.Denom())
	output.SetInt(output.Num())
	return output.Mul(output, &wallet.lot)
}

/* sweep prices displayed depth without spending unavailable cash. */
func (wallet *virtualWallet) sweep(book *spotbook.Book, requested *big.Rat, buy bool) (quantity, gross *big.Rat) {
	quantity, gross = new(big.Rat), new(big.Rat)
	level := book.Bids.High

	if buy {
		level = book.Asks.Low
	}

	var remaining, available, fill, cost, affordable big.Rat
	remaining.Set(requested)
	available.Set(&wallet.cash)

	for level != nil && remaining.Sign() > 0 {
		price := level.Price.Rat()
		fill.Set(level.Quantity.Rat())

		if remaining.Cmp(&fill) < 0 {
			fill.Set(&remaining)
		}

		if buy {
			cost.Mul(price, &wallet.factor)
			affordable.Quo(&available, &cost)
			wallet.floor(&affordable, &affordable)

			// Asks ascend in price. If one lot is unaffordable here, every
			// remaining ask is unaffordable too.
			if affordable.Sign() == 0 {
				break
			}

			if affordable.Cmp(&fill) < 0 {
				fill.Set(&affordable)
			}
		}

		cost.Mul(price, &fill)
		quantity.Add(quantity, &fill)
		gross.Add(gross, &cost)
		remaining.Sub(&remaining, &fill)

		if buy {
			cost.Mul(&cost, &wallet.factor)
			available.Sub(&available, &cost)
			level = level.Higher
			continue
		}

		level = level.Lower
	}

	return quantity, gross
}

/* mark includes full inventory liquidation at visible bids and its exit fee. */
func (wallet *virtualWallet) mark(book *spotbook.Book) (*big.Rat, bool) {
	if wallet.quantity.Sign() == 0 {
		return new(big.Rat).Set(&wallet.cash), true
	}
	quantity, gross := wallet.sweep(book, &wallet.quantity, false)

	if quantity.Cmp(&wallet.quantity) != 0 {
		return nil, false
	}

	var fee big.Rat
	fee.Mul(gross, &wallet.fee)
	gross.Sub(gross, &fee)
	return gross.Add(gross, &wallet.cash), true
}

/* maximum derives executable quantity from cash, inventory and depth. */
func (wallet *virtualWallet) maximum(book *spotbook.Book, buy bool) *big.Rat {
	if !buy {
		return new(big.Rat).Set(&wallet.quantity)
	}
	requested := new(big.Rat).Quo(&wallet.cash, book.Asks.Low.Price.Rat())
	wallet.floor(requested, requested)
	quantity, _ := wallet.sweep(book, requested, true)
	return wallet.floor(quantity, quantity)
}

/* actions enumerates feasible dyadic quantities without a selected allocation. */
func (wallet *virtualWallet) actions(book *spotbook.Book, output []LearningAction) []LearningAction {
	output = append(output[:0], LearningAction{Kind: types.ActionHold})
	var previous, cost big.Rat

	for _, buy := range []bool{true, false} {
		quantity := wallet.maximum(book, buy)
		kind, price := types.ActionScale, book.Asks.Low.Price.Rat()

		if wallet.quantity.Sign() == 0 {
			kind = types.ActionEnter
		}
		if !buy {
			price = book.Bids.High.Price.Rat()
		}

		previous.SetInt64(0)

		for power := uint16(0); quantity.Sign() > 0; power++ {
			cost.Mul(quantity, price)

			if quantity.Cmp(&wallet.minimum) < 0 || cost.Cmp(&wallet.costMinimum) < 0 {
				break
			}

			action := LearningAction{Kind: kind, Power: power, Reduce: !buy}

			if !buy && power == 0 {
				action.Kind = types.ActionExit
			}
			if quantity.Cmp(&previous) != 0 {
				output = append(output, action)
			}

			previous.Set(quantity)
			// Two is the numerical bisection radix, not an allocation multiplier.
			quantity.Quo(quantity, big.NewRat(2, 1))
			wallet.floor(quantity, quantity)
		}
	}

	return output
}

/* request fixes quantity before execution evidence, scaled by input authority. */
func (wallet *virtualWallet) request(book *spotbook.Book, action LearningAction, authority float64) *big.Rat {
	if action.Kind == types.ActionHold {
		return new(big.Rat)
	}
	quantity := wallet.maximum(book, !action.Reduce)

	for range action.Power {
		quantity.Quo(quantity, big.NewRat(2, 1))
		wallet.floor(quantity, quantity)
	}

	if !action.Reduce {
		var influence big.Rat
		influence.SetFloat64(authority)
		quantity.Mul(quantity, &influence)
		wallet.floor(quantity, quantity)
	}

	return quantity
}

/* fill applies a pending IOC, cancels the unfilled remainder and charges fees. */
func (wallet *virtualWallet) fill(book *spotbook.Book, action LearningAction, requested *big.Rat) (quantity, gross, fee *big.Rat) {
	quantity, gross, fee = new(big.Rat), new(big.Rat), new(big.Rat)
	price := book.Asks.Low.Price.Rat()

	if action.Reduce {
		price = book.Bids.High.Price.Rat()
	}

	var cost big.Rat
	cost.Mul(requested, price)

	if action.Kind == types.ActionHold || requested.Cmp(&wallet.minimum) < 0 || cost.Cmp(&wallet.costMinimum) < 0 {
		return quantity, gross, fee
	}

	quantity, gross = wallet.sweep(book, requested, !action.Reduce)
	fee.Mul(gross, &wallet.fee)
	wallet.fees.Add(&wallet.fees, fee)

	if action.Reduce {
		cost.Sub(gross, fee)
		wallet.cash.Add(&wallet.cash, &cost)
		wallet.quantity.Sub(&wallet.quantity, quantity)
		return quantity, gross, fee
	}

	cost.Add(gross, fee)
	wallet.cash.Sub(&wallet.cash, &cost)
	wallet.quantity.Add(&wallet.quantity, quantity)
	return quantity, gross, fee
}

/*
restart clones the same known starting capital into a spent account. Venue
rules, fee schedule and formatting scale are unchanged: this is the same
account economics beginning a new episode, not a different instrument. The
fees consumed by the finished episode are returned so a lane can retain what
its exploration actually cost across episodes.
*/
func (wallet *virtualWallet) restart(initial *decimal.Decimal) *big.Rat {
	spent := new(big.Rat).Set(&wallet.fees)
	wallet.cash.Set(initial.Rat())
	wallet.quantity.SetInt64(0)
	wallet.fees.SetInt64(0)
	return spent
}
