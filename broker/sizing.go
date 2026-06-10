package broker

import (
	"errors"
	"fmt"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

var (
	ErrNoPosition        = errors.New("broker: no open position")
	ErrNoMark            = errors.New("broker: no mark price")
	ErrInsufficientCash  = errors.New("broker: insufficient quote cash")
	ErrNegativeQuoteCash = errors.New("broker: negative quote cash")
)

/*
SizeOrder converts a playbook fraction into an executable quantity using live
cash, inventory, and mark.
*/
func SizeOrder(
	action *logic.Action,
	risk RiskContext,
	entryScale float64,
	quoteCash float64,
	heldQty float64,
	mark float64,
	constraints *krakenmarket.InstrumentConstraints,
) (float64, error) {
	if action == nil {
		return 0, fmt.Errorf("broker: nil action")
	}

	if action.Fraction <= 0 {
		return 0, fmt.Errorf("broker: action fraction must be positive")
	}

	if action.Fraction > 1 {
		return 0, fmt.Errorf("broker: action fraction must not exceed 1")
	}

	var (
		quantity float64
		sizeErr  error
	)

	if action.Type.IsExit() {
		quantity, sizeErr = sizeExit(action, heldQty)
	} else {
		quantity, sizeErr = sizeEntry(action, risk, entryScale, quoteCash, mark)
	}

	if sizeErr != nil {
		return 0, sizeErr
	}

	if constraints == nil {
		return quantity, nil
	}

	return ApplyInstrumentConstraints(action, quantity, mark, *constraints)
}

func sizeExit(action *logic.Action, heldQty float64) (float64, error) {
	if heldQty > 0 {
		quantity := heldQty * action.Fraction

		if quantity <= 0 {
			return 0, fmt.Errorf("broker: exit quantity rounds to zero")
		}

		return quantity, nil
	}

	if heldQty < 0 {
		quantity := -heldQty * action.Fraction

		if quantity <= 0 {
			return 0, fmt.Errorf("broker: exit quantity rounds to zero")
		}

		return quantity, nil
	}

	return 0, ErrNoPosition
}

func sizeEntry(
	action *logic.Action,
	risk RiskContext,
	entryScale float64,
	quoteCash float64,
	mark float64,
) (float64, error) {
	if quoteCash < 0 {
		return 0, ErrNegativeQuoteCash
	}

	if mark <= 0 {
		return 0, ErrNoMark
	}

	if risk.PositionFraction <= 0 {
		return 0, fmt.Errorf("broker: trading.position_fraction must be positive")
	}

	if entryScale <= 0 {
		return 0, fmt.Errorf("broker: entry size scale must be positive")
	}

	notional := quoteCash * risk.PositionFraction * action.Fraction * entryScale

	if notional <= 0 {
		return 0, ErrInsufficientCash
	}

	quantity := notional / mark

	if quantity <= 0 {
		return 0, fmt.Errorf("broker: entry quantity rounds to zero")
	}

	return quantity, nil
}
