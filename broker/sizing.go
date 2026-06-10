package broker

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

var (
	ErrNoPosition       = errors.New("broker: no open position")
	ErrNoMark           = errors.New("broker: no mark price")
	ErrInsufficientCash = errors.New("broker: insufficient quote cash")
)

/*
SizeOrder converts a playbook fraction into an executable quantity using live
cash, inventory, and mark.
*/
func SizeOrder(
	action *logic.Action,
	quoteCash float64,
	heldQty float64,
	mark float64,
) (float64, error) {
	if action == nil {
		return 0, fmt.Errorf("broker: nil action")
	}

	if action.Fraction <= 0 {
		return 0, fmt.Errorf("broker: action fraction must be positive")
	}

	switch {
	case action.Type.IsExit() || action.Side == trading.Sell:
		if heldQty <= 0 {
			return 0, ErrNoPosition
		}

		quantity := heldQty * action.Fraction

		if quantity <= 0 {
			return 0, fmt.Errorf("broker: exit quantity rounds to zero")
		}

		return quantity, nil
	default:
		if mark <= 0 {
			return 0, ErrNoMark
		}

		positionFraction := viper.GetFloat64("trading.position_fraction")

		if positionFraction <= 0 {
			return 0, fmt.Errorf("broker: trading.position_fraction must be positive")
		}

		notional := quoteCash * positionFraction * action.Fraction

		if notional <= 0 {
			return 0, ErrInsufficientCash
		}

		quantity := notional / mark

		if quantity <= 0 {
			return 0, fmt.Errorf("broker: entry quantity rounds to zero")
		}

		return quantity, nil
	}
}
