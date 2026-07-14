package strategy

import (
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
It refuses incomplete lifecycles and advances only PostMortem-ready Theses.
*/
func (postMortem *PostMortem) Evaluate(
	thesis *types.Thesis,
	symbol string,
) error {
	if thesis.LifecycleState(symbol) != types.LifecyclePostMortemReady {
		return errnie.Err(errnie.Validation, "Thesis is not PostMortem-ready for "+symbol, nil)
	}

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

	accepted := false
	reconciledEntry := false
	buyFilled := false
	sellFilled := false
	closed := false
	final := types.TradeObservation{}
	executionIDs := make([]string, 0)
	evaluatedAt := time.Time{}

	for _, observation := range thesis.TradeJournal {
		if observation.Symbol != symbol {
			continue
		}

		if observation.Kind == "broker_acceptance" && observation.Action == "enter" {
			accepted = true
		}

		if observation.Kind == "position_reconciliation" {
			reconciledEntry = true
			executionIDs = append(executionIDs, observation.ExecutionID)
		}

		if observation.Kind == "execution" && observation.Side == "buy" {
			buyFilled = true
			executionIDs = append(executionIDs, observation.ExecutionID)
		}

		if observation.Kind == "execution" && observation.Side == "sell" {
			sellFilled = true
			executionIDs = append(executionIDs, observation.ExecutionID)
		}

		if observation.Kind == "position_snapshot" &&
			observation.Status == types.LifecycleClosed {
			quantity, err := decimal.NewFromString(observation.Quantity)

			if err != nil {
				return errnie.Err(errnie.Validation, "invalid closed quantity for "+symbol, err)
			}

			closed = quantity.Sign() == 0
		}

		if observation.Kind == "final_outcome" {
			final = observation
		}

		if observation.At.After(evaluatedAt) {
			evaluatedAt = observation.At
		}
	}

	if entry == nil && !reconciledEntry {
		return errnie.Err(errnie.Validation, "entry decision or reconciliation required for "+symbol, nil)
	}

	if exit == nil || (!accepted && !reconciledEntry) || !sellFilled || !closed ||
		final.PnL == "" || final.Fee == "" {
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
			Component: "forecast", Condition: "entry forecast selected for execution",
			Evidence: []string{entry.ForecastSource, entry.ForecastModel,
				strconv.FormatUint(entry.ForecastEpoch, 10)},
			EstimatedEffect: entry.ExpectedReturn, Uncertainty: entry.Uncertainty,
			RequiredValidation: validation, CurrentModel: entry.ForecastModel,
		},
		types.Finding{
			Component: "decision", Condition: entry.Reason,
			Evidence:        []string{entry.Action, exit.Action, exit.Reason},
			EstimatedEffect: entry.Utility, Uncertainty: entry.Uncertainty,
			RequiredValidation: validation,
		},
		types.Finding{
			Component: "execution", Condition: "entry and exit fills reconciled with reported fees",
			Evidence: executionIDs, EstimatedEffect: final.ReturnPct,
			RequiredValidation: validation,
		},
	)

	return thesis.Transition(symbol, types.LifecycleEvaluated, evaluatedAt)
}
