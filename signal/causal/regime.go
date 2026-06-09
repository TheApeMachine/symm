package causal

import (
	"math"

	"github.com/spf13/viper"
)

const (
	regimeNormal = "flow"
	regimePanic  = "liquidity"
)

/*
causalRoles assigns DAG node roles for one structural regime.

In the normal regime macro momentum and liquidity are backdoor controls and local flow is the
intervention whose effect on price velocity we read. When a dominant maker weaponises liquidity
— pulling quotes so the sudden void itself drives price while order flow merely lags into it —
the edges invert: liquidity becomes the treatment, and local flow drops out of the control set
because conditioning on what has become a mediator would block the very effect we want to
measure. The two role sets are pre-computed; switching between them is a branch, not a
re-discovery of the graph, so it costs nothing on the hot path.
*/
type causalRoles struct {
	treatment int
	controls  []int
	label     string
}

func normalRoles() causalRoles {
	return causalRoles{
		treatment: localFlowNode,
		controls:  []int{macroMomentumNode, liquidityNode},
		label:     regimeNormal,
	}
}

func panicRoles() causalRoles {
	return causalRoles{
		treatment: liquidityNode,
		controls:  []int{macroMomentumNode},
		label:     regimePanic,
	}
}

func (roles causalRoles) predictors() []int {
	return append(append([]int(nil), roles.controls...), roles.treatment)
}

func selectRolesFromTable(
	nodeTable dagNodeTable,
	contagion float64,
) (causalRoles, bool, float64) {
	normal := normalRoles()

	v := viper.GetViper()
	contagionBreakThreshold := v.GetFloat64("signals.causal.contagion_break")
	conditionSwitch := v.GetFloat64("signals.causal.condition_switch")

	contagionBreak := contagionBreakThreshold > 0 &&
		contagion >= contagionBreakThreshold

	condition := 0.0
	conditionBreak := false

	if conditionSwitch > 0 {
		pairCondition, condErr := nodeTable.PairConditionNumber(liquidityNode, localFlowNode)

		if condErr == nil {
			condition = pairCondition
			conditionBreak = math.IsInf(pairCondition, 1) ||
				pairCondition >= conditionSwitch
		}
	}

	if conditionBreak || contagionBreak {
		return panicRoles(), true, condition
	}

	return normal, false, condition
}

func selectRolesWithTracker(
	nodeTable dagNodeTable,
	contagion float64,
	tracker *regimeTracker,
	historyLen int,
) (causalRoles, bool, float64) {
	_, rawInverted, condition := selectRolesFromTable(nodeTable, contagion)

	if tracker == nil {
		if rawInverted {
			return panicRoles(), true, condition
		}

		return normalRoles(), false, condition
	}

	inverted := tracker.apply(rawInverted, deriveRegimeHysteresisSamples(historyLen))

	if inverted {
		return panicRoles(), true, condition
	}

	return normalRoles(), false, condition
}
