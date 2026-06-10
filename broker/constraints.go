package broker

import (
	"fmt"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

/*
ApplyInstrumentConstraints quantizes quantity and validates exchange minimums.
*/
func ApplyInstrumentConstraints(
	action *logic.Action,
	quantity float64,
	mark float64,
	constraints krakenmarket.InstrumentConstraints,
) (float64, error) {
	if quantity <= 0 {
		return 0, fmt.Errorf("broker: quantity must be positive")
	}

	if constraints.QtyIncrement <= 0 {
		return 0, fmt.Errorf("broker: missing qty increment for %s", constraints.Symbol)
	}

	quantized, quantErr := krakenmarket.QuantizeDown(quantity, constraints.QtyIncrement)

	if quantErr != nil {
		return 0, quantErr
	}

	if constraints.QtyMin > 0 && quantized < constraints.QtyMin {
		return 0, fmt.Errorf("broker: quantity below minimum for %s", constraints.Symbol)
	}

	if constraints.CostMin > 0 && mark > 0 {
		cost := quantized * mark

		if cost < constraints.CostMin {
			return 0, fmt.Errorf("broker: notional below minimum for %s", constraints.Symbol)
		}
	}

	if action != nil && action.Type.IsExit() {
		return quantized, nil
	}

	return quantized, nil
}
