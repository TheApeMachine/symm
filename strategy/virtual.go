package strategy

import (
	"math"
	"math/big"

	"github.com/theapemachine/symm/broker"

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
	cash, quantity, fees big.Rat
	pricing              broker.Pricing
	pair                 kraken.InstrumentPair
	scale                int
}

/* initialize fixes the account's starting capital and supplied venue rules. */
func (wallet *virtualWallet) initialize(initial *decimal.Decimal, pair kraken.InstrumentPair, fee *decimal.Decimal) error {
	wallet.pair = pair
	wallet.cash.Set(initial.Rat())
	if err := wallet.pricing.Configure(pair, fee); err != nil {
		return err
	}
	// A decimal product needs the sum of its operand scales. Percent conversion
	// adds two decimal places. This bounds exact journal formatting, not math.
	wallet.scale = int(max(initial.GetScale(), int64(pair.PricePrecision+pair.QtyPrecision)+fee.GetScale()+2))
	return nil
}

/* mark includes full inventory liquidation at visible bids and its exit fee. */
func (wallet *virtualWallet) mark(book *spotbook.Book) (*big.Rat, bool) {
	if wallet.quantity.Sign() == 0 {
		return new(big.Rat).Set(&wallet.cash), true
	}
	quantity, gross := wallet.pricing.Sweep(book, &wallet.quantity, &wallet.cash, false, nil, nil)

	if quantity.Cmp(&wallet.quantity) != 0 {
		return nil, false
	}

	wallet.pricing.Total(gross, gross, false)
	return gross.Add(gross, &wallet.cash), true
}

/* maximum derives executable quantity from cash, inventory and depth. */
func (wallet *virtualWallet) maximum(book *spotbook.Book, buy bool) *big.Rat {
	if !buy {
		return new(big.Rat).Set(&wallet.quantity)
	}
	requested := wallet.pricing.Affordable(&wallet.cash, book.Asks.Low.Price.Rat())
	wallet.pricing.Floor(requested, requested)
	quantity, _ := wallet.pricing.Sweep(book, requested, &wallet.cash, true, nil, nil)
	return wallet.pricing.Floor(quantity, quantity)
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
			broker.NotionalRat(&cost, price, quantity)

			if quantity.Cmp(&wallet.pricing.Minimum) < 0 || cost.Cmp(&wallet.pricing.CostMinimum) < 0 {
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
			wallet.pricing.Floor(quantity, quantity)
		}
	}

	return output
}

/*
request fixes quantity before execution evidence, scaled by input authority,
and records the depth it was sized against. That ladder is what the later fill
is measured against: an execution may only take liquidity that was displayed
when the decision was made and is still displayed when it runs.
*/
func (wallet *virtualWallet) request(
	book *spotbook.Book, action LearningAction, authority float64, observed *broker.DepthLadder,
) *big.Rat {
	if action.Kind == types.ActionHold {
		return new(big.Rat)
	}
	quantity := wallet.maximum(book, !action.Reduce)

	for range action.Power {
		quantity.Quo(quantity, big.NewRat(2, 1))
		wallet.pricing.Floor(quantity, quantity)
	}

	if !action.Reduce {
		var influence big.Rat
		influence.SetFloat64(authority)
		quantity.Mul(quantity, &influence)
		wallet.pricing.Floor(quantity, quantity)
	}

	if observed != nil {
		observed.Count = 0
		wallet.pricing.Sweep(book, quantity, &wallet.cash, !action.Reduce, observed, nil)
	}

	return quantity
}

/*
fill applies a pending IOC against the next book, cancels the unfilled
remainder and charges fees. The observed ladder is the depth the decision was
sized against: each level is capped by what stood there then, so liquidity that
arrived afterwards is not credited and liquidity that has since been cancelled
is not taken. That is the race an order actually runs; an unrestricted sweep of
the current book wins it every time and drifts the lanes optimistic.
*/
func (wallet *virtualWallet) fill(
	book *spotbook.Book, action LearningAction, requested *big.Rat, observed *broker.DepthLadder,
) (quantity, gross, fee *big.Rat) {
	quantity, gross, fee = new(big.Rat), new(big.Rat), new(big.Rat)
	price := book.Asks.Low.Price.Rat()

	if action.Reduce {
		price = book.Bids.High.Price.Rat()
	}

	var cost big.Rat
	broker.NotionalRat(&cost, price, requested)

	if action.Kind == types.ActionHold || requested.Cmp(&wallet.pricing.Minimum) < 0 || cost.Cmp(&wallet.pricing.CostMinimum) < 0 {
		return quantity, gross, fee
	}

	quantity, gross = wallet.pricing.Sweep(book, requested, &wallet.cash, !action.Reduce, nil, observed)
	wallet.pricing.Fee(fee, gross)
	wallet.fees.Add(&wallet.fees, fee)

	if action.Reduce {
		wallet.pricing.Total(&cost, gross, false)
		wallet.cash.Add(&wallet.cash, &cost)
		wallet.quantity.Sub(&wallet.quantity, quantity)
		return quantity, gross, fee
	}

	wallet.pricing.Total(&cost, gross, true)
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

/* context conditions market structure on this wallet's own executable exposure. */
func (wallet *virtualWallet) context(sequence []uint64, book *spotbook.Book, equity float64, output []uint64) []uint64 {
	output = append(output[:0], sequence...)
	exposure := uint64(0)

	if wallet.quantity.Sign() > 0 && equity > 0 {
		gross, _ := broker.NotionalRat(new(big.Rat), book.Bids.High.Price.Rat(), &wallet.quantity).Float64()
		fraction := gross / equity

		if fraction > 0 {
			exposure = uint64(max(0, -math.Floor(math.Log2(fraction)))) + 1
		}
	}
	return append(output, 0, exposure)
}
