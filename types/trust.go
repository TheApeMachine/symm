package types

import (
	"math"
	"time"

)

/*
computeObservationTrust is the node's influence mass Ω ∈ [0,1]:

	Ω = Maturity × HypothesisSeparation × Freshness × Skill

Signals emit Maturity and HypothesisSeparation. Freshness is age since At
scaled by the observation's own Horizon (At − ObservedFrom). Skill applies
only when a predictive coder published it. There is no invented strength or
confidence term: a young, ambiguous, or stale reading simply carries little
mass.
*/
/*
ObservationMass is the public name for the node's Ω used at graph compile.
*/
func ObservationMass(node *Node, now time.Time) float64 {
	return computeObservationTrust(node, now)
}

func computeObservationTrust(node *Node, now time.Time) float64 {
	if node == nil || node.Kind == "" {
		return 0
	}

	trust := nodeMaturity(node) * nodeSeparation(node) *
		nodeFreshness(node, now) * nodeSkill(node)

	return math.Max(0, math.Min(1, trust))
}

/*
nodeSeparation is the margin between competing hypotheses. Measurements that
never stamped one honestly report zero — nothing has been shown to stand out.
Non-measurement nodes have no rival-kernel census, so absence is neutral (1).
*/
func nodeSeparation(node *Node) float64 {
	if node == nil {
		return 0
	}

	separation, found := node.Metadata["hypothesis_separation"].(float64)

	if !found {
		if node.Kind == KindMeasurement {
			return 0
		}

		return 1
	}

	if math.IsNaN(separation) || math.IsInf(separation, 0) {
		if node.Kind == KindMeasurement {
			return 0
		}

		return 1
	}

	return math.Max(0, math.Min(1, separation))
}

/*
nodeMaturity is the estimator's own filled fraction. Measurements start at
zero and rise as support accumulates. Other kinds have no warmup census, so
an unset maturity is treated as fully present rather than invented.
*/
func nodeMaturity(node *Node) float64 {
	if node == nil {
		return 0
	}

	if node.Maturity <= 0 {
		if node.Kind == KindMeasurement {
			return 0
		}

		return 1
	}

	if math.IsNaN(node.Maturity) || math.IsInf(node.Maturity, 0) {
		if node.Kind == KindMeasurement {
			return 0
		}

		return 1
	}

	return math.Max(0, math.Min(1, node.Maturity))
}

/*
nodeFreshness is exp(−age / τ) where τ is the observation window that produced
the node (Horizon, or At − ObservedFrom). Without a window there is no
memory scale to invent, so the reading is treated as current.
*/
func nodeFreshness(node *Node, now time.Time) float64 {
	if node == nil || node.At.IsZero() || now.IsZero() {
		return 1
	}

	elapsed := now.Sub(node.At).Seconds()

	if elapsed < 0 {
		elapsed = 0
	}

	horizon := node.Horizon.Seconds()

	if horizon <= 0 && !node.ObservedFrom.IsZero() &&
		!node.At.Before(node.ObservedFrom) {
		horizon = node.At.Sub(node.ObservedFrom).Seconds()
	}

	if horizon <= 0 {
		return 1
	}

	return math.Exp(-elapsed / horizon)
}

/*
nodeSkill reads the prequential skill published by the predictive coder. Skill
below its own baseline attenuates trust; at or above baseline it becomes a
non-negative confidence factor capped so a single lucky run cannot multiply
the observation.
*/
func nodeSkill(node *Node) float64 {
	skill, found := node.Metadata["task_skill"].(float64)

	if !found {
		return 1
	}

	if math.IsNaN(skill) || math.IsInf(skill, 0) {
		return 1
	}

	if skill <= 0 {
		return 0
	}

	return math.Min(1, skill/(skill+1)*2)
}
