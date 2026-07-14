package strategy

import (
	"errors"
	"strconv"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
PostMortem evaluates one completed Thesis without mutating a live model. Its
findings preserve forecast, decision, and realized execution effects separately
so later aggregation can test systematic adjustments rather than chase a trade.
*/
type PostMortem struct{}

/*
Evaluate verifies the complete round trip and records evidence-backed findings.
It advances valid PostMortem-ready Theses to evaluated and marks immutable,
incomplete journals invalid so the runtime cannot retry them indefinitely.
*/
func (postMortem *PostMortem) Evaluate(
	thesis *types.Thesis,
	symbol string,
) (err error) {
	if thesis.LifecycleState(symbol) != types.LifecyclePostMortemReady {
		return errnie.Err(errnie.Validation, "Thesis is not PostMortem-ready for "+symbol, nil)
	}

	defer func() {
		if err == nil {
			return
		}

		err = errors.Join(
			err,
			thesis.Transition(symbol, types.LifecycleInvalid, time.Now()),
		)
	}()

	var entry, exit *types.Decision

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Symbol != symbol {
			continue
		}

		if decision.Action == "enter" && entry == nil {
			entry = decision
		}

		if decision.Action == "exit" {
			exit = decision
		}
	}

	var entryAcceptance, reconciliation, buyFill, sellFill, closure, final *types.TradeObservation
	evaluatedAt := time.Time{}

	for index := range thesis.TradeJournal {
		observation := &thesis.TradeJournal[index]

		if observation.Symbol != symbol {
			continue
		}

		if observation.At.After(evaluatedAt) {
			evaluatedAt = observation.At
		}

		switch {
		case observation.Kind == "broker_acceptance" && observation.Action == "enter":
			entryAcceptance = postMortem.latestTradeObservation(entryAcceptance, observation)
		case observation.Kind == "position_reconciliation":
			reconciliation = postMortem.latestTradeObservation(reconciliation, observation)
		case observation.Kind == "execution" && observation.Side == "buy":
			buyFill = postMortem.latestTradeObservation(buyFill, observation)
		case observation.Kind == "execution" && observation.Side == "sell":
			sellFill = postMortem.latestTradeObservation(sellFill, observation)
		case observation.Kind == "position_snapshot" &&
			observation.Status == types.LifecycleClosed:
			closure = postMortem.latestTradeObservation(closure, observation)
		case observation.Kind == "final_outcome":
			final = postMortem.latestTradeObservation(final, observation)
		}
	}

	accepted := entryAcceptance != nil
	reconciledEntry := reconciliation != nil
	buyFilled := buyFill != nil
	sellFilled := sellFill != nil
	closed := false

	if closure != nil {
		quantity, err := decimal.NewFromString(closure.Quantity)

		if err != nil {
			return errnie.Err(errnie.Validation, "invalid closed quantity for "+symbol, err)
		}

		closed = quantity.Sign() == 0
	}

	executionIDs := make([]string, 0, 3)

	if reconciliation != nil && reconciliation.ExecutionID != "" {
		executionIDs = append(executionIDs, reconciliation.ExecutionID)
	}

	if buyFill != nil && buyFill.ExecutionID != "" {
		executionIDs = append(executionIDs, buyFill.ExecutionID)
	}

	if sellFill != nil && sellFill.ExecutionID != "" {
		executionIDs = append(executionIDs, sellFill.ExecutionID)
	}

	if entry == nil && !reconciledEntry {
		return errnie.Err(errnie.Validation, "entry decision or reconciliation required for "+symbol, nil)
	}

	if exit == nil || (!accepted && !reconciledEntry) || !sellFilled || !closed ||
		final == nil || final.PnL == "" || final.Fee == "" {
		return errnie.Err(errnie.Validation, "incomplete reconciled trade journal for "+symbol, nil)
	}

	if !postMortem.tradeJournalOrdered(
		entryAcceptance, reconciliation, buyFill, sellFill, closure, final,
	) {
		return errnie.Err(errnie.Validation, "incomplete reconciled trade journal for "+symbol, nil)
	}

	if _, err := decimal.NewFromString(final.PnL); err != nil {
		return errnie.Err(errnie.Validation, "invalid realized PnL for "+symbol, err)
	}

	if _, err := decimal.NewFromString(final.Fee); err != nil {
		return errnie.Err(errnie.Validation, "invalid realized fees for "+symbol, err)
	}

	validation := "aggregate comparable completed Theses and validate by chronological replay"

	if reconciledEntry && entry == nil {
		thesis.Findings = append(thesis.Findings, types.Finding{
			Symbol:    symbol,
			Component: "reconciliation",
			Condition: "position existed before the strategy decision journal",
			Evidence:  executionIDs, EstimatedEffect: final.ReturnPct,
			RequiredValidation: validation,
		})

		return thesis.Transition(symbol, types.LifecycleEvaluated, evaluatedAt)
	}

	if !buyFilled {
		return errnie.Err(errnie.Validation, "entry fill required for "+symbol, nil)
	}

	thesis.Findings = append(thesis.Findings,
		types.Finding{
			Symbol:    symbol,
			Component: "forecast",
			Condition: "entry forecast selected for execution",
			Evidence: []string{
				entry.ForecastSource,
				entry.ForecastModel,
				strconv.FormatUint(entry.ForecastEpoch, 10),
			},
			EstimatedEffect:    entry.ExpectedReturn,
			Uncertainty:        entry.Uncertainty,
			RequiredValidation: validation,
			CurrentModel:       entry.ForecastModel,
		},
		types.Finding{
			Symbol:    symbol,
			Component: "decision",
			Condition: entry.Reason,
			Evidence: []string{
				entry.Action, exit.Action, exit.Reason,
			},
			EstimatedEffect:    entry.Utility,
			Uncertainty:        entry.Uncertainty,
			RequiredValidation: validation,
		},
		types.Finding{
			Symbol:             symbol,
			Component:          "execution",
			Condition:          "entry and exit fills reconciled with reported fees",
			Evidence:           executionIDs,
			EstimatedEffect:    final.ReturnPct,
			RequiredValidation: validation,
		},
	)

	return thesis.Transition(symbol, types.LifecycleEvaluated, evaluatedAt)
}

/*
latestTradeObservation keeps the journal event with the latest At timestamp.
*/
func (postMortem *PostMortem) latestTradeObservation(
	current, candidate *types.TradeObservation,
) *types.TradeObservation {
	if candidate == nil {
		return current
	}

	if current == nil || !candidate.At.Before(current.At) {
		return candidate
	}

	return current
}

/*
tradeJournalOrdered verifies entry, exit, closure, and final outcome timestamps
advance in chronological order using the latest event selected for each stage.
*/
func (postMortem *PostMortem) tradeJournalOrdered(
	entryAcceptance, reconciliation, buyFill, sellFill, closure, final *types.TradeObservation,
) bool {
	entryAt := postMortem.tradeJournalEntryAt(entryAcceptance, reconciliation)

	if entryAt.IsZero() || sellFill == nil || closure == nil || final == nil {
		return false
	}

	previous := entryAt

	if buyFill != nil {
		if previous.After(buyFill.At) {
			return false
		}

		previous = buyFill.At
	}

	if previous.After(sellFill.At) || sellFill.At.After(closure.At) || closure.At.After(final.At) {
		return false
	}

	return true
}

/*
tradeJournalEntryAt returns the earliest accepted entry boundary for the trade.
*/
func (postMortem *PostMortem) tradeJournalEntryAt(
	entryAcceptance, reconciliation *types.TradeObservation,
) time.Time {
	entryAt := time.Time{}

	if reconciliation != nil {
		entryAt = reconciliation.At
	}

	if entryAcceptance != nil &&
		(entryAt.IsZero() || entryAcceptance.At.Before(entryAt)) {
		entryAt = entryAcceptance.At
	}

	return entryAt
}
