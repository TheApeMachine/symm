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
func AssertMeasure(theses []*types.Thesis, entry Entry) error {
	if entry.Truth.MeasureSource == "" || entry.Truth.MeasureMetric == "" {
		return nil
	}

	bound := tests.BoundPositive

	switch entry.Truth.MeasureBound {
	case "present":
		bound = tests.BoundPresent
	case "zero":
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
func AssertDecide(thesis *types.Thesis, entry Entry) error {
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
AssertWallet checks preserve/deploy cash bounds after strategy.
*/
func AssertWallet(before float64, after float64, entry Entry) error {
	switch entry.Truth.WalletBound {
	case "preserve":
		if after < before-0.01 {
			return fmt.Errorf(
				"catalog %s: wallet should preserve cash (before=%g after=%g)",
				entry.Name, before, after,
			)
		}
	case "deploy":
		if after >= before {
			return fmt.Errorf(
				"catalog %s: wallet should deploy cash (before=%g after=%g)",
				entry.Name, before, after,
			)
		}
	}

	return nil
}
