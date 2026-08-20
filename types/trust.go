package types

import (
	"math"
	"time"
)

/*
computeObservationTrust returns the epistemic trust coefficient Ω ∈ [0,1] for
one node: the degree to which this observation deserves to drive a decision.

Ω = Separation × Maturity × TemporalFreshness × Skill

Every factor is measured from the node's own provenance; an absent factor
contributes its neutral value (1) rather than silently zeroing the observation,
so only a genuinely degenerate reading — a separation that is literally muddy,
a maturity floor that fails, a stale timestamp, or below-baseline skill —
attenuates it.
*/
func computeObservationTrust(node *Node, now time.Time) float64 {
	if node == nil || node.Confidence <= 0 {
		return 0
	}

	separation := nodeSeparation(node)
	maturity := nodeMaturity(node)
	freshness := nodeFreshness(node, now)
	skill := nodeSkill(node)

	trust := node.Confidence * separation * maturity * freshness * skill
	return math.Max(0, math.Min(1, trust))
}

/*
nodeSeparation reads the competing-hypothesis sharpness stored by a signal as
hypothesis_separation in the node's metadata. When a signal did not publish the
separation, the reading is treated as unambiguously separated (1): the absence
of a competing score is not evidence of ambiguity.
*/
func nodeSeparation(node *Node) float64 {
	separation, found := node.Metadata["hypothesis_separation"].(float64)

	if !found {
		return 1
	}

	if math.IsNaN(separation) || math.IsInf(separation, 0) {
		return 1
	}

	return math.Max(0, math.Min(1, separation))
}

/*
nodeMaturity converts the signal's estimator maturity into a confidence factor.
Maturity is already the fraction of a signal's own capacity the estimator has
observed, so it is used directly. A provisional observation that never reported
its maturity is kept at a defensible floor rather than trusted fully.
*/
func nodeMaturity(node *Node) float64 {
	if node.Maturity <= 0 {
		return 0.1
	}

	if math.IsNaN(node.Maturity) || math.IsInf(node.Maturity, 0) {
		return 0.1
	}

	return math.Max(0, math.Min(1, node.Maturity))
}

/*
nodeFreshness attenuates an observation as its natural relaxation time elapses.
Each source owns a decay half-life matched to its physical timescale: order-flow
touch reads die in under a second, lead-lag survives tens of seconds, and the
fluid/cognitive fields relax over minutes. A node with no timestamp is treated
as fresh (1) because staleness cannot be established without a clock.
*/
func nodeFreshness(node *Node, now time.Time) float64 {
	if node == nil || node.At.IsZero() || now.IsZero() {
		return 1
	}

	halfLife := nodeHalflife(node)
	elapsed := now.Sub(node.At).Seconds()

	if elapsed < 0 {
		elapsed = 0
	}

	return math.Exp(-elapsed / halfLife)
}

/*
nodeHalflife is the physical relaxation time of one observation source. The
half-lives are stated in the source's own event-time: a touch-level depth or
arrival read ages in hundreds of milliseconds, a volume-clock pump reading in
seconds, cross-symbol dispersion in tens of seconds, and the manifold/cognition
fields in minutes.
*/
func nodeHalflife(node *Node) float64 {
	if node == nil {
		return 10
	}

	switch node.Kind {
	case KindManifold:
		return 30
	case KindCognition:
		return 60
	}

	switch node.Source {
	case "depthflow", "hawkes", "toxicity":
		return 0.5
	case "cvd", "pumpdump":
		return 3
	case "leadlag", "sentiment":
		return 15
	case "liquidity", "correlation", "exhaustion":
		return 8
	default:
		return 10
	}
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
