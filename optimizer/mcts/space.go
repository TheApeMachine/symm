package mcts

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

var (
	searchObservations = []perspectives.ObservationType{
		perspectives.ObservationNone,
		perspectives.ObservationHolding,
		perspectives.ObservationNotHolding,
	}

	searchUnits = []perspectives.UnitType{
		perspectives.UnitSNR,
		perspectives.UnitConfidence,
	}

	searchConditions = []perspectives.ConditionType{
		perspectives.ConditionIsGreaterThanOrEqual,
		perspectives.ConditionIsLessThanOrEqual,
		perspectives.ConditionIsGreaterThan,
		perspectives.ConditionIsLessThan,
	}

	searchEntryActions = []perspectives.ActionType{
		perspectives.ActionNone,
		perspectives.ActionLimit,
		perspectives.ActionMarket,
		perspectives.ActionIceberg,
	}

	searchExitActions = []perspectives.ActionType{
		perspectives.ActionNone,
		perspectives.ActionStopLoss,
		perspectives.ActionStopLossLimit,
		perspectives.ActionTakeProfit,
		perspectives.ActionTakeProfitLimit,
		perspectives.ActionTrailingStop,
		perspectives.ActionTrailingStopLimit,
		perspectives.ActionSettlePosition,
	}
)

/*
regimeSearchSpace returns the regimes the MCTS search conditions branches on.

With optimizer.search.regime_aware enabled (the default) it explores the full
price-action regime set, so the optimizer can discover "in regime X do Y" trees;
RegimeNone is always included so regime-agnostic branches stay reachable.
Disabled, it collapses to {RegimeNone} — the legacy behaviour and a far smaller
move set. The regime a branch gates on is computed identically here, in replay,
and live (perspectives.ClassifyRegime), so a discovered regime edge reproduces.
*/
func regimeSearchSpace() []perspectives.Regime {
	if !regimeAwareSearch() {
		return []perspectives.Regime{perspectives.RegimeNone}
	}

	return []perspectives.Regime{
		perspectives.RegimeNone,
		perspectives.RegimeDead,
		perspectives.RegimeChoppy,
		perspectives.RegimeTrending,
		perspectives.RegimeBullish,
		perspectives.RegimeBearish,
	}
}

func regimeAwareSearch() bool {
	if !viper.IsSet("optimizer.search.regime_aware") {
		return true
	}

	return viper.GetBool("optimizer.search.regime_aware")
}
