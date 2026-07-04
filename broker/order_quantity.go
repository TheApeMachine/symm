package broker

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

func (factory *OrderFactory) quantity(
	action *datura.Artifact,
	balances *BalanceBook,
	quote MarketQuote,
	seed orderSeed,
) (float64, float64, error) {
	if actionQuantity := actionFloat(action, "quantity"); actionQuantity > 0 {
		return factory.cappedQuantity(actionQuantity, balances, seed)
	}

	if seed.side == "sell" || seed.actionType.IsExit() {
		available, err := balances.RequireFunds(baseAsset(seed.symbol, factory.quote))
		if err != nil {
			return 0, 0, err
		}

		if available <= 0 {
			return 0, 0, errnie.Error(errnie.Err(errnie.NotAcceptable, "broker: no base balance to sell for "+seed.symbol, nil))
		}

		return available, 0, nil
	}

	cash, err := balances.RequireFunds(factory.quote)
	if err != nil {
		return 0, 0, err
	}

	notional := factory.notional(action, cash)
	if notional <= 0 {
		return 0, 0, errnie.Error(errnie.Err(errnie.Validation, "broker: buy action has no notional or fraction for "+seed.symbol, nil))
	}

	price := quote.Price(seed.side)
	if price <= 0 {
		return 0, 0, errnie.Error(errnie.Err(errnie.Validation, "broker: quote price unavailable for "+seed.symbol, nil))
	}

	quantity := notional / price
	if quantity <= 0 || math.IsNaN(quantity) || math.IsInf(quantity, 0) {
		return 0, 0, errnie.Error(errnie.Err(errnie.Validation, "broker: computed non-positive quantity for "+seed.symbol, nil))
	}

	return quantity, notional, nil
}

func (factory *OrderFactory) notional(action *datura.Artifact, cash float64) float64 {
	fraction := actionFloat(action, "fraction")
	if fraction <= 0 {
		fraction = actionFloat(action, "risk", "fraction")
	}

	notional := actionFloat(action, "notional")
	if notional <= 0 && fraction > 0 {
		notional = cash * fraction
	}

	if notional > cash {
		return cash
	}

	return notional
}

func (factory *OrderFactory) cappedQuantity(
	quantity float64,
	balances *BalanceBook,
	seed orderSeed,
) (float64, float64, error) {
	if seed.side != "sell" && !seed.actionType.IsExit() {
		return quantity, 0, nil
	}

	available, err := balances.RequireFunds(baseAsset(seed.symbol, factory.quote))
	if err != nil {
		return 0, 0, err
	}

	if quantity > available {
		quantity = available
	}

	if quantity <= 0 {
		return 0, 0, errnie.Error(errnie.Err(errnie.NotAcceptable, "broker: no sell quantity available for "+seed.symbol, nil))
	}

	return quantity, 0, nil
}
