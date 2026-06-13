package market

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
)

const defaultEntryFeeBps = 26.0

/*
prepareAction stamps symbol, sizes entries against the capital envelope, and
fills exit quantities from holdings.
*/
func prepareAction(
	ctx context.Context,
	holdings *logic.Holdings,
	occupancy logic.EntrySlotOccupancy,
	action *logic.Action,
	measurements []logic.Measurement,
	tradingConfig config.TradingConfig,
	thresholdCtx logic.ThresholdContext,
	capitalProvider trader.CapitalProvider,
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
		return prepareExitAction(&stamped, holdings, measurements)
	}

	if stamped.Side != trading.Buy {
		return &stamped, nil
	}

	if holdings.IsHolding(stamped.Symbol) {
		return nil, nil
	}

	executionCost := logic.ExecutionCostFromMarket(
		measurements,
		defaultEntryFeeBps,
		0,
		0,
	)

	if !logic.MeetsExpectedEdgeGate(measurements, executionCost) {
		return nil, nil
	}

	candidate, candidateOk := logic.BestEntryCandidate(measurements, executionCost)

	if !candidateOk || !logic.EntryCandidateLong(candidate) {
		return nil, nil
	}

	qualifiesForOpportunity := logic.QualifiesForOpportunityEntryFromCandidate(
		candidate,
		thresholdCtx,
	)

	allowed, opportunitySlot := logic.EntrySlotAdmission(
		occupancy,
		tradingConfig,
		qualifiesForOpportunity,
	)

	if !allowed {
		return nil, nil
	}

	entryConfidence := stamped.EntryConfidence

	if candidateOk {
		entryConfidence = candidate.Confidence
	}

	modelFraction, err := trader.EntrySlotFraction(
		holdings,
		occupancy,
		measurements,
		executionCost,
		candidate,
		thresholdCtx,
		tradingConfig,
		opportunitySlot,
	)

	if err != nil {
		return nil, errnie.Error(err)
	}

	strategyCap := 1.0

	if stamped.Fraction > 0 {
		strategyCap = stamped.Fraction
	}

	fraction := math.Min(modelFraction, strategyCap)

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

	if len(candidate.Sources) > 0 {
		stamped.ReasonSource = candidate.Sources[0]
	}

	if len(candidate.Categories) > 0 {
		stamped.ReasonCategory = candidate.Categories[0]
	}

	return &stamped, nil
}

func prepareExitAction(
	action *logic.Action,
	holdings *logic.Holdings,
	_ []logic.Measurement,
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

	exitFraction := action.Fraction
	exitTier := action.ExitTier

	if exitTier == 0 && action.ReasonCategory != logic.CategoryTypeNone {
		exitTier = logic.ExitTierForCategory(action.ReasonCategory)
	}

	if exitTier != 0 {
		exitFraction = logic.ExitFractionForTier(exitTier, action.Fraction)
	}

	if exitFraction > 0 && exitFraction < 1 {
		quantity *= exitFraction
	}

	action.Quantity = quantity

	return action, nil
}
