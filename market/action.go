package market

import (
	"errors"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
)

/*
prepareAction stamps symbol, sizes entries against the capital envelope, and
fills exit quantities from holdings.
*/
func prepareAction(
	holdings *logic.Holdings,
	action *logic.Action,
	measurements []logic.Measurement,
	tradingConfig config.TradingConfig,
	paperWalletQuote float64,
) (*logic.Action, error) {
	if action == nil {
		return nil, nil
	}

	stamped := *action

	if stamped.Symbol == "" {
		symbol, err := logic.SymbolFromMeasurements(measurements)

		if err != nil {
			return nil, errnie.Error(err)
		}

		stamped.Symbol = symbol
	}

	if stamped.Type.IsExit() {
		return prepareExitAction(&stamped, holdings)
	}

	if stamped.Side != trading.Buy {
		return &stamped, nil
	}

	if holdings.IsHolding(stamped.Symbol) {
		return nil, nil
	}

	if holdings.OpenCount() >= tradingConfig.MaxConcurrentPositions {
		return nil, nil
	}

	entryConfidence := logic.PeakConfidence(measurements)

	fraction, err := trader.EntrySlotFraction(holdings, entryConfidence, tradingConfig)

	if err != nil {
		return nil, errnie.Error(err)
	}

	price, err := logic.ReferencePrice(measurements)

	if err != nil {
		return nil, errnie.Error(err)
	}

	walletQuote := paperWalletQuote

	if tradingConfig.Model != "paper" {
		var walletErr error

		walletQuote, walletErr = trader.QuoteWalletBalance(tradingConfig.Model)

		if walletErr != nil {
			return nil, errnie.Error(walletErr)
		}
	}

	if walletQuote <= 0 {
		return nil, errnie.Error(fmt.Errorf(
			"market: wallet quote balance must be positive",
		))
	}

	quantity, err := trader.OrderQuantityFromFraction(walletQuote, fraction, price)

	if err != nil {
		return nil, errnie.Error(err)
	}

	stamped.Fraction = fraction
	stamped.Quantity = quantity
	stamped.Price = price
	stamped.EntryConfidence = entryConfidence

	return &stamped, nil
}

func prepareExitAction(
	action *logic.Action,
	holdings *logic.Holdings,
) (*logic.Action, error) {
	if holdings == nil {
		return nil, errnie.Error(errors.New("market: holdings are required for exits"))
	}

	quantity := holdings.Quantity(action.Symbol)

	if quantity <= 0 {
		return nil, errnie.Error(fmt.Errorf(
			"market: exit requested without open position for %s",
			action.Symbol,
		))
	}

	if action.Fraction > 0 && action.Fraction < 1 {
		quantity *= action.Fraction
	}

	action.Quantity = quantity

	return action, nil
}
