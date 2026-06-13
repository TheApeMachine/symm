package market

import (
	"context"
	"errors"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
)

/*
prepareAction stamps symbol, sizes entries against the capital envelope, and
fills exit quantities from holdings.
*/
func prepareAction(
	ctx context.Context,
	holdings *logic.Holdings,
	action *logic.Action,
	measurements []logic.Measurement,
	tradingConfig config.TradingConfig,
	thresholdConfig config.ThresholdConfig,
	capitalProvider trader.CapitalProvider,
	regimeVolatility float64,
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

	qualifiesForOpportunity := logic.QualifiesForOpportunityEntry(
		measurements,
		thresholdConfig,
	)

	allowed, opportunitySlot := logic.EntrySlotAdmission(
		holdings,
		tradingConfig,
		qualifiesForOpportunity,
	)

	if !allowed {
		return nil, nil
	}

	entryConfidence := logic.PeakConfidence(measurements)

	fraction, err := trader.EntrySlotFraction(
		holdings,
		measurements,
		thresholdConfig,
		tradingConfig,
		regimeVolatility,
		opportunitySlot,
	)

	if err != nil {
		return nil, errnie.Error(err)
	}

	price, err := logic.ReferencePrice(measurements)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if capitalProvider == nil {
		return nil, errnie.Error(errors.New(
			"market: capital provider is required for entry sizing",
		))
	}

	_, quote, err := krakenmarket.SplitPairSymbol(stamped.Symbol)

	if err != nil {
		return nil, errnie.Error(err)
	}

	walletQuote, err := capitalProvider.AvailableQuoteBalance(ctx, quote)

	if err != nil {
		return nil, errnie.Error(err)
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
	stamped.OpportunitySlot = opportunitySlot

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
