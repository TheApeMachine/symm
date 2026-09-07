package strategy

import (
	"math"

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
	quantity, gross, err := wallet.price.Sweep(book, wallet.quantity, nil, broker.SELL, nil, nil)

	if err != nil || quantity.Cmp(wallet.quantity) != 0 {
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
	quantity, _, err := wallet.price.Sweep(book, requested, wallet.cash, broker.BUY, nil, nil)
	return quantity, err
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

/* request fixes quantity and observed depth before later execution evidence. */
func (wallet *virtualWallet) request(
	book *spotbook.Book, action LearningAction, authority float64, observed *broker.DepthLadder,
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

	if observed != nil {
		observed.Count = 0
		side := broker.BUY

		if action.Reduce {
			side = broker.SELL
		}
		_, _, err = wallet.price.Sweep(book, quantity, wallet.cash, side, observed, nil)
	}
	return quantity, err
}

/* fill cancels unfilled IOC quantity and accounts only for surviving depth. */
func (wallet *virtualWallet) fill(
	book *spotbook.Book, action LearningAction, requested *decimal.Decimal, observed *broker.DepthLadder,
) (quantity, gross, fee *decimal.Decimal, err error) {
	unit, side := book.BestAsk().Price, broker.BUY

	if action.Reduce {
		unit, side = book.BestBid().Price, broker.SELL
	}

	if action.Kind == types.ActionHold || !wallet.price.Tradable(wallet.symbol, requested, unit) {
		return zero, zero, zero, nil
	}
	quantity, gross, err = wallet.price.Sweep(book, requested, wallet.cash, side, nil, observed)

	if err != nil {
		return nil, nil, nil, err
	}
	total := wallet.price.WithFee(wallet.symbol, gross, side)
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

/* context represents exposure as a learning feature, without changing money. */
func (wallet *virtualWallet) context(sequence []uint64, book *spotbook.Book, equity float64, output []uint64) []uint64 {
	output = append(output[:0], sequence...)
	exposure := uint64(0)

	if wallet.quantity.Sign() > 0 && equity > 0 {
		fraction := wallet.quantity.Float64() * book.BestBid().Price.Float64() / equity

		if fraction > 0 {
			exposure = uint64(max(0, -math.Floor(math.Log2(fraction)))) + 1
		}
	}
	return append(output, 0, exposure)
}
