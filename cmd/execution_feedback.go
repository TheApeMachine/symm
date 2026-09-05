package cmd

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/strategy"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

/* executionFeedback owns action-correlated venue outcomes and realization evidence. */
type executionFeedback struct {
	submitted   atomic.Uint64
	realization *strategy.RealizationMeter
	inFlight    sync.Map
	funds       *accountFunds
}

/* AttachRealization connects the agent's circuit breaker to authoritative venue feedback. */
func (feedback *executionFeedback) AttachRealization(meter *strategy.RealizationMeter) {
	if feedback == nil {
		return
	}

	feedback.realization = meter
}

/*
RecordLifecycle receives authoritative broker execution facts (fills, closes) and
reports realized execution slippage to RealizationMeter.
*/
func (feedback *executionFeedback) RecordLifecycle(event hindsight.LifecycleEvent) {
	if feedback == nil || event.ActionCorrelationID == "" || event.Execution == nil {
		return
	}

	value, found := feedback.inFlight.Load(event.ActionCorrelationID)

	if event.Kind == "execution_submitted" {
		if found {
			feedback.submitted.Add(1)
		}

		if found && feedback.realization != nil {
			feedback.realization.ObserveSubmission(nil)
		}

		return
	}

	switch event.Kind {
	case "execution_refused":
		feedback.inFlight.Delete(event.ActionCorrelationID)
		feedback.funds.Release(event.ActionCorrelationID, time.Time{})
		return
	case "execution_terminal", "execution_failed":
		feedback.funds.Release(event.ActionCorrelationID, time.Now().UTC())
		value, found = feedback.inFlight.LoadAndDelete(event.ActionCorrelationID)
	case "entry_fill", "increase_fill", "reduce_fill", "exit_fill":
	default:
		return
	}

	if !found || feedback.realization == nil {
		return
	}

	intent := value.(strategy.ExecutionIntent)

	if event.Kind == "execution_failed" || event.Execution.OrderStatus == "rejected" {
		feedback.realization.ObserveSubmission(errnie.Err(
			errnie.IO, "symm: execution failed for "+intent.Symbol, nil,
		))
	}

	if event.Execution.CumQty == "" || event.Execution.CumQty == "0" {
		return
	}

	if intent.Reference == nil {
		errnie.Error(errnie.Err(errnie.Validation, "symm: fill intent missing reference", nil))
		return
	}

	refPrice, _ := intent.Reference.Float64()

	if refPrice <= 0 {
		errnie.Error(errnie.Err(errnie.Validation, "symm: fill reference must be positive", nil))
		return
	}

	// Whole-order cumulative economics are equivalent to the venue's average.
	// LastPrice alone is not the average of a multi-fill order.
	fillPriceStr := event.Execution.AvgPrice

	if fillPriceStr == "" {
		cost, costOK := new(big.Rat).SetString(event.Execution.CumCost)
		quantity, quantityOK := new(big.Rat).SetString(event.Execution.CumQty)

		if !costOK || !quantityOK || quantity.Sign() <= 0 {
			errnie.Error(errnie.Err(errnie.Validation, "symm: fill missing cumulative economics", nil))
			return
		}

		fillPrice, _ := cost.Quo(cost, quantity).Float64()
		feedback.realization.ObserveFill(refPrice, fillPrice, intent.Reduce)
		return
	}

	fillPrice, err := strconv.ParseFloat(fillPriceStr, 64)

	if err != nil || fillPrice <= 0 {
		errnie.Error(errnie.Err(errnie.Validation, "symm: invalid average fill price", err))
		return
	}

	reduce := intent.Reduce
	feedback.realization.ObserveFill(refPrice, fillPrice, reduce)
}
