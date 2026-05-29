package causal

import (
	"math"

	"github.com/theapemachine/symm/config"
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

/*
selectRoles chooses the structural regime for the current sample history. The normal regime
holds unless severe instability is detected through either channel the critique identified:

  - liquidity and local flow collapsing onto a single axis — the condition number of their 2x2
    correlation matrix exploding, the linear-algebra signature of the two edges no longer being
    separately identifiable; or
  - a cross-asset contagion break, where the Hayashi-Yoshida correlation across the universe
    spikes toward one as a liquidation cascade drags every venue together.

Either trip flips the engine to the pre-computed panic roles. The boolean reason it returns is
empty in the normal regime and names the inversion otherwise, so the measurement can explain
itself downstream.
*/
func selectRoles(samples []causalSample, contagion float64) (causalRoles, bool) {
	normal := normalRoles()

	contagionBreak := config.System.CausalContagionBreak > 0 &&
		contagion >= config.System.CausalContagionBreak

	conditionBreak := false

	if config.System.CausalConditionSwitch > 0 {
		if nodeTable, err := causalTable(samples); err == nil {
			condition, condErr := nodeTable.PairConditionNumber(liquidityNode, localFlowNode)

			if condErr == nil {
				conditionBreak = math.IsInf(condition, 1) ||
					condition >= config.System.CausalConditionSwitch
			}
		}
	}

	if conditionBreak || contagionBreak {
		return panicRoles(), true
	}

	return normal, false
}
