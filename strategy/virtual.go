package strategy

import (
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/* LearningAction identifies an intervention and its binary quantity refinement. */
type LearningAction struct {
	Kind   types.Action `json:"kind"`
	Power  uint16       `json:"power"`
	Reduce bool         `json:"reduce"`
}

var (
	zero = decimal.NewFromInt64(0)
	two  = decimal.NewFromInt64(2)
)

/*
fillAffordabilityPasses bounds how many times a buy is re-sized against the
cost it actually walked to. Each pass scales by the measured shortfall and the
walked cost is monotone in size, so it converges quickly; the bound exists so a
pathological book cannot spin here rather than because convergence is in doubt.
*/
const fillAffordabilityPasses = 8

/* virtualWallet accounts for one independent taker IOC experiment in Decimal. */
type virtualWallet struct {
	cash, quantity, fees *decimal.Decimal
	price                *broker.Price
	symbol               string
}

func (wallet *virtualWallet) initialize(initial *decimal.Decimal, price *broker.Price, symbol string) error {
	if price.FeeIfAvailable(symbol) == nil {
		return errnie.Error(errnie.Err(errnie.NotFound, "virtual wallet: fee unavailable for "+symbol, nil))
	}
	wallet.price, wallet.symbol = price, symbol
	wallet.cash, wallet.quantity, wallet.fees = initial.SetScale(decimal.DefaultScale), zero, zero
	return nil
}

/* mark includes full visible inventory liquidation and its fee. */
func (wallet *virtualWallet) mark(book *spotbook.Book) (*decimal.Decimal, bool, error) {
	if wallet.quantity.Sign() == 0 {
		return wallet.cash, true, nil
	}
	quantity, gross, err := wallet.price.Walk(book, wallet.quantity, broker.SELL)

	if quantity == nil || quantity.Cmp(wallet.quantity) != 0 {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}
	return wallet.cash.Add(wallet.price.WithFee(wallet.symbol, gross, broker.SELL)), true, nil
}

func (wallet *virtualWallet) maximum(book *spotbook.Book, buy bool) (*decimal.Decimal, error) {
	if !buy {
		return wallet.quantity, nil
	}
	requested, err := wallet.price.Affordable(wallet.symbol, wallet.cash, book.BestAsk().Price)

	if err != nil {
		return nil, err
	}

	if requested.Sign() <= 0 {
		return requested, nil
	}

	quantity, _, err := wallet.price.Walk(book, requested, broker.BUY)

	if quantity != nil {
		return quantity, nil
	}

	return nil, err
}

/* actions enumerates feasible quantity bisections down to the actual venue minimum. */
func (wallet *virtualWallet) actions(book *spotbook.Book, output []LearningAction) ([]LearningAction, error) {
	output = append(output[:0], LearningAction{Kind: types.ActionHold})
	pair := wallet.price.Instrument.Pair(wallet.symbol)

	for _, buy := range []bool{true, false} {
		quantity, err := wallet.maximum(book, buy)

		if err != nil {
			return nil, err
		}
		kind, unit := types.ActionScale, book.BestAsk().Price

		if wallet.quantity.Sign() == 0 {
			kind = types.ActionEnter
		}

		if !buy {
			unit = book.BestBid().Price
		}
		previous := zero

		for power := uint16(0); wallet.price.Tradable(wallet.symbol, quantity, unit); power++ {
			action := LearningAction{Kind: kind, Power: power, Reduce: !buy}

			if !buy && power == 0 {
				action.Kind = types.ActionExit
			}

			if quantity.Cmp(previous) != 0 {
				output = append(output, action)
			}
			previous = quantity
			quantity = quantity.Div(two).SetSize(pair.QtyIncrement)
		}
	}
	return output, nil
}

/* request fixes quantity before later execution evidence. */
func (wallet *virtualWallet) request(
	book *spotbook.Book, action LearningAction, authority float64,
) (*decimal.Decimal, error) {
	if action.Kind == types.ActionHold {
		return zero, nil
	}
	quantity, err := wallet.maximum(book, !action.Reduce)

	if err != nil {
		return nil, err
	}
	pair := wallet.price.Instrument.Pair(wallet.symbol)

	for range action.Power {
		quantity = quantity.Div(two).SetSize(pair.QtyIncrement)
	}

	if !action.Reduce {
		quantity = quantity.Mul(decimal.NewFromFloat64(authority)).SetSize(pair.QtyIncrement)
	}
	return quantity, nil
}

/*
fill cancels unfilled IOC quantity and accounts only for surviving depth.

A request is sized when the decision issues and reaches the book at least one
update later, so the price it is filled at is not the price it was sized
against. A buy is therefore re-capped against the cash actually held at the
touch it is filling on: the wallet spends what it has and the remainder is
cancelled, which is what the venue would do with the unaffordable part. Without
that cap a request sized on a cheaper book overdraws the wallet, and every
figure derived from its equity afterwards is measuring an account that could
not have existed.
*/
func (wallet *virtualWallet) fill(
	book *spotbook.Book, action LearningAction, requested *decimal.Decimal,
) (quantity, gross, fee *decimal.Decimal, err error) {
	unit, side := book.BestAsk().Price, broker.BUY

	if action.Reduce {
		unit, side = book.BestBid().Price, broker.SELL
	}

	if action.Kind == types.ActionHold {
		return zero, zero, zero, nil
	}

	if !action.Reduce {
		affordable, err := wallet.price.Affordable(wallet.symbol, wallet.cash, unit)

		if err != nil {
			return nil, nil, nil, err
		}

		if requested.Cmp(affordable) > 0 {
			requested = affordable
		}
	}

	if !wallet.price.Tradable(wallet.symbol, requested, unit) {
		return zero, zero, zero, nil
	}
	quantity, gross, err = wallet.price.Walk(book, requested, side)

	if quantity == nil {
		return nil, nil, nil, err
	}

	if quantity.Sign() == 0 {
		return zero, zero, zero, nil
	}
	total := wallet.price.WithFee(wallet.symbol, gross, side)

	/*
		The touch price only bounds the first level. A quantity that is
		affordable against the touch can still cost more once it has walked
		into the levels behind it, so the walked total — the number the wallet
		actually pays — is what has to fit the cash.

		Each pass scales the request by the shortfall it just measured and
		walks again, which converges because the walked cost rises with size.
		A request that still does not fit is cancelled outright rather than
		part-filled at a price the wallet could not have paid.
	*/
	for attempt := 0; !action.Reduce && total.Cmp(wallet.cash) > 0; attempt++ {
		if attempt == fillAffordabilityPasses {
			return zero, zero, zero, nil
		}
		pair := wallet.price.Instrument.Pair(wallet.symbol)
		requested = requested.Mul(wallet.cash).Div(total).SetSize(pair.QtyIncrement)

		if !wallet.price.Tradable(wallet.symbol, requested, unit) {
			return zero, zero, zero, nil
		}
		quantity, gross, err = wallet.price.Walk(book, requested, side)

		if quantity == nil {
			return nil, nil, nil, err
		}

		if quantity.Sign() == 0 {
			return zero, zero, zero, nil
		}
		total = wallet.price.WithFee(wallet.symbol, gross, side)
	}
	fee = total.Sub(gross).Abs()
	wallet.fees = wallet.fees.Add(fee)

	if action.Reduce {
		wallet.cash = wallet.cash.Add(total)
		wallet.quantity = wallet.quantity.Sub(quantity)
		return quantity, gross, fee, nil
	}
	wallet.cash = wallet.cash.Sub(total)
	wallet.quantity = wallet.quantity.Add(quantity)
	return quantity, gross, fee, nil
}

/* restart retains all charged fees and starts another independent experiment. */
func (wallet *virtualWallet) restart(initial *decimal.Decimal) *decimal.Decimal {
	spent := wallet.fees
	wallet.cash, wallet.quantity, wallet.fees = initial.SetScale(decimal.DefaultScale), zero, zero
	return spent
}

/* state reports whether the wallet holds open inventory or flat cash. */
func (wallet *virtualWallet) state() string {
	if wallet.quantity.Sign() == 0 {
		return "flat"
	}

	return "holding"
}
