package catalog

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
AssertMeasure checks that the tape produced the expected signal evidence.
*/
func (entry Entry) AssertMeasure(theses []*types.Thesis) error {
	if entry.Truth.MeasureSource == "" || entry.Truth.MeasureMetric == "" {
		return nil
	}

	bound := tests.BoundPositive

	switch entry.Truth.MeasureBound {
	case MeasureBoundPresent:
		bound = tests.BoundPresent
	case MeasureBoundZero:
		bound = tests.BoundZero
	}

	if err := tests.CheckSourceClaim(theses, tests.SourceClaim{
		Source: entry.Truth.MeasureSource,
		Metric: entry.Truth.MeasureMetric,
		Symbol: entry.Symbol,
		Bound:  bound,
	}); err != nil {
		return fmt.Errorf("catalog %s: %w", entry.Name, err)
	}

	return nil
}

/*
AssertDecide checks enter/refuse and sizing against StageTruth after CommitStrategy.
*/
func (entry Entry) AssertDecide(thesis *types.Thesis) error {
	if thesis == nil {
		return fmt.Errorf("catalog %s: nil thesis after strategy", entry.Name)
	}

	var enter *types.Decision

	for index := range thesis.Decisions {
		decision := &thesis.Decisions[index]

		if decision.Symbol != entry.Symbol {
			continue
		}

		if decision.Action == types.ActionEnter {
			enter = decision
			break
		}
	}

	if entry.Truth.MustNotEnter {
		if enter != nil && enter.ProposedQuantity != nil && enter.ProposedQuantity.Sign() > 0 {
			return fmt.Errorf(
				"catalog %s: must not enter %s (got qty %s)",
				entry.Name, entry.Symbol, enter.ProposedQuantity,
			)
		}

		return nil
	}

	if entry.Truth.DecideAction == types.ActionEnter {
		if enter == nil {
			return fmt.Errorf("catalog %s: want enter on %s", entry.Name, entry.Symbol)
		}

		if entry.Truth.SizedEnter {
			if enter.ProposedQuantity == nil || enter.ProposedQuantity.Sign() <= 0 {
				return fmt.Errorf(
					"catalog %s: want sized enter, reason=%s",
					entry.Name, enter.Reason,
				)
			}

			if enter.ProposedNotional == nil || enter.ProposedNotional.Sign() <= 0 {
				return fmt.Errorf("catalog %s: want proposed notional", entry.Name)
			}

			fraction := viper.GetFloat64("trading.allocation.max_fraction")
			ceiling := entry.Capital * fraction

			if enter.ProposedNotional.Float64() > ceiling+1e-6 {
				return fmt.Errorf(
					"catalog %s: notional %s exceeds max_fraction slice %g",
					entry.Name, enter.ProposedNotional, ceiling,
				)
			}
		}
	}

	return nil
}

/*
AssertExit checks open-lot Regulate outcomes after CommitStrategy.
*/
func (entry Entry) AssertExit(thesis *types.Thesis, lot *types.Holding) error {
	if thesis == nil {
		return fmt.Errorf("catalog %s: nil thesis after exit regulate", entry.Name)
	}

	cause := ""

	for _, decision := range thesis.Decisions {
		if decision.Symbol == entry.Symbol && decision.Action == types.ActionExit {
			cause = decision.Cause
			break
		}
	}

	if entry.Truth.MustNotExit {
		if cause != "" {
			return fmt.Errorf(
				"catalog %s: must hold open lot, got exit cause %q (action=%s reason=%s)",
				entry.Name, cause, lotAction(lot), lotReason(lot),
			)
		}

		if lot != nil && lot.Stoploss != nil &&
			(lot.Stoploss.Action == "stop" || lot.Stoploss.Action == "take_profit") {
			return fmt.Errorf(
				"catalog %s: stoploss action %q under MustNotExit (%s)",
				entry.Name, lot.Stoploss.Action, lot.Stoploss.Reason,
			)
		}

		if lot != nil && lot.Stoploss != nil && lot.Stoploss.TrailDistance <= 0 {
			return fmt.Errorf(
				"catalog %s: open lot armed without positive TrailDistance",
				entry.Name,
			)
		}

		if entry.Truth.MinStopReturn > 0 && (lot == nil || lot.Stoploss == nil) {
			return fmt.Errorf("catalog %s: want stoploss for MinStopReturn", entry.Name)
		}

		if entry.Truth.MinStopReturn > 0 &&
			lot.Stoploss.StopReturn < entry.Truth.MinStopReturn {
			return fmt.Errorf(
				"catalog %s: StopReturn %g below MinStopReturn %g",
				entry.Name, lot.Stoploss.StopReturn, entry.Truth.MinStopReturn,
			)
		}

		if entry.Truth.RequireLockedFloor &&
			(lot == nil || lot.Stoploss == nil || lot.Stoploss.LockedFloor <= 0) {
			return fmt.Errorf(
				"catalog %s: want positive LockedFloor after calibrated mark",
				entry.Name,
			)
		}

		return nil
	}

	if entry.Truth.ExitCause == "" {
		return nil
	}

	if cause != entry.Truth.ExitCause {
		return fmt.Errorf(
			"catalog %s: want exit cause %q, got %q (stoploss=%s/%s)",
			entry.Name, entry.Truth.ExitCause, cause, lotAction(lot), lotReason(lot),
		)
	}

	return nil
}

func lotAction(lot *types.Holding) string {
	if lot == nil || lot.Stoploss == nil {
		return ""
	}

	return lot.Stoploss.Action
}

func lotReason(lot *types.Holding) string {
	if lot == nil || lot.Stoploss == nil {
		return ""
	}

	return lot.Stoploss.Reason
}

/*
AssertWallet checks preserve/deploy cash bounds after strategy.
*/
func (entry Entry) AssertWallet(before float64, after float64) error {
	switch entry.Truth.WalletBound {
	case WalletBoundPreserve:
		if after < before-0.01 {
			return fmt.Errorf(
				"catalog %s: wallet should preserve cash (before=%g after=%g)",
				entry.Name, before, after,
			)
		}
	case WalletBoundDeploy:
		if after >= before {
			return fmt.Errorf(
				"catalog %s: wallet should deploy cash (before=%g after=%g)",
				entry.Name, before, after,
			)
		}
	}

	return nil
}
