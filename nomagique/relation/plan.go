package relation

import "time"

/*
Selector is a structural coordinate selector. An empty field is a wildcard
for that identity component. Selectors are wiring, not name magic: they bind
exact coordinates into explicit Source/Target/Control roles.
*/
type Selector struct {
	// Source is the signal source, e.g. "cvd".
	Source string
	// Metric is the metric name; empty selects all metrics of Source.
	Metric string
	// Side is the side suffix; empty selects both sides.
	Side string
}

/*
Matches reports whether a coordinate satisfies the selector.
*/
func (selector Selector) Matches(coordinate Coordinate) bool {
	if selector.Source != "" && selector.Source != coordinate.Source {
		return false
	}

	if selector.Metric != "" && selector.Metric != coordinate.Metric {
		return false
	}

	if selector.Side != "" && selector.Side != coordinate.Side {
		return false
	}

	return true
}

/*
ControlSelector is one explicit control in a RelationPlan: a coordinate
selector plus the alignment lag for that control. A zero lag aligns the
control at the same cutoff as the Source (t - sourceLag); a positive lag
aligns it at t - controlLag, which is required to condition on a mediator at
the time slice that actually blocks a path.
*/
type ControlSelector struct {
	Selector
	Lag time.Duration
}

/*
PlannedPair is one structurally eligible Source→Target pair in a RelationPlan.
*/
type PlannedPair struct {
	Source Selector
	Target Selector
}

/*
LagDomain is the candidate lag search domain expressed in time. A zero
MinLag falls back to the derived lag resolution; a zero MaxLag is bounded by
the retained history (infrastructure provenance, published as LagSearchSpan).
*/
type LagDomain struct {
	MinLag time.Duration
	MaxLag time.Duration
}

/*
RelationPlan is the explicit typed plan that defines which Relations are
eligible. Eligibility is structural only: symbol scope, peer scope, explicit
pairs, and exact controls. It never depends on current evidence values — a
valid low-gain or zero-gain Relation remains eligible and representable.
*/
type RelationPlan struct {
	// Version is the relation-plan version; it participates in the model
	// epoch contract.
	Version uint64
	// Epoch is the model epoch this plan belongs to.
	Epoch uint64
	// Symbol is the symbol scope; empty means any symbol.
	Symbol string
	// Peer is the peer scope; empty means no peer restriction.
	Peer string
	// Pairs enumerates the explicit Source→Target pairs to estimate.
	Pairs []PlannedPair
	// Sources and Targets define the cross-product candidate space: every
	// Source coordinate × every Target coordinate (self-pairs excluded).
	// This is how a plan declares "all configured same-symbol compatible
	// coordinate pairs" without enumerating every combination.
	Sources []Selector
	Targets []Selector
	// Controls are the explicit structural controls applied to every pair.
	Controls []ControlSelector
	// Lag is the candidate lag domain.
	Lag LagDomain
}

/*
PairsForSymbol returns the planned pairs applicable to one symbol, or nil
when the plan's scope excludes it. Cross-product pairs are expanded
structurally; self-pairs (identical Source and Target coordinates) are
excluded because Influence requires a positive lag between distinct
coordinates.
*/
func (plan *RelationPlan) PairsForSymbol(symbol string) []PlannedPair {
	if plan == nil {
		return nil
	}

	if plan.Symbol != "" && plan.Symbol != symbol {
		return nil
	}

	pairs := make([]PlannedPair, 0, len(plan.Pairs)+len(plan.Sources)*len(plan.Targets))
	pairs = append(pairs, plan.Pairs...)

	for _, source := range plan.Sources {
		for _, target := range plan.Targets {
			if source == target {
				continue
			}

			pairs = append(pairs, PlannedPair{Source: source, Target: target})
		}
	}

	return pairs
}

/*
ResolveControls resolves the plan's control selectors against the stored
coordinates available for the symbol, returning explicit controls in selector
order. The boolean reports whether every exact control selector resolved to a
stored coordinate. A wildcard selector resolves to every matching stored
coordinate; this is structural availability, not evidence. A missing exact
control makes the Relation unavailable rather than silently changing the
model.
*/
/*
ResolveControls resolves the plan's control selectors against the resident
coordinates available for the symbol, returning explicit controls in selector
order. The boolean reports whether every exact control selector resolved to a
registered coordinate. A wildcard selector resolves to every matching
resident coordinate; this is structural availability, not evidence. A missing
exact control makes the Relation unavailable rather than silently changing
the model.
*/
func (plan *RelationPlan) ResolveControls(symbol string, store *ObservationStore) ([]Control, bool) {
	if plan == nil {
		return nil, true
	}

	controls := make([]Control, 0, len(plan.Controls))

	for _, selector := range plan.Controls {
		matched := false

		store.RangeCoordinatesForSymbol(symbol, func(coordinate Coordinate) bool {
			if plan.Peer != "" && coordinate.Peer != plan.Peer {
				return true
			}

			if !selector.Matches(coordinate) {
				return true
			}

			controls = append(controls, Control{Coordinate: coordinate, Lag: selector.Lag})
			matched = true
			return true
		})

		// An exact selector (any identity component populated) with no
		// matching coordinate is a missing control: the Relation is
		// unavailable rather than silently changing the model.
		exact := selector.Source != "" || selector.Metric != "" || selector.Side != ""

		if !matched && exact {
			return nil, false
		}
	}

	return controls, true
}

/*
CompiledCandidate represents a pre-resolved (Source, Target, Controls, Lag) candidate pair.
*/
type CompiledCandidate struct {
	Plan             *RelationPlan
	Source           Coordinate
	Target           Coordinate
	Controls         []Control
	ControlsComplete bool
	Lag              LagDomain
}

/*
CompilePlansForSymbol precompiles the relation candidates across all active plans
for a symbol against the store's resident coordinates.
*/
func CompilePlansForSymbol(
	plans []*RelationPlan,
	symbol string,
	epoch uint64,
	store *ObservationStore,
) []CompiledCandidate {
	var candidates []CompiledCandidate

	for _, plan := range plans {
		if plan == nil || plan.Epoch != epoch {
			continue
		}

		controls, controlsComplete := plan.ResolveControls(symbol, store)

		for _, pair := range plan.PairsForSymbol(symbol) {
			sources := resolveSelectorsForSymbol(pair.Source, symbol, plan.Peer, epoch, store)
			targets := resolveSelectorsForSymbol(pair.Target, symbol, plan.Peer, epoch, store)

			for _, source := range sources {
				for _, target := range targets {
					if source == target {
						continue
					}

					candidates = append(candidates, CompiledCandidate{
						Plan:             plan,
						Source:           source,
						Target:           target,
						Controls:         controls,
						ControlsComplete: controlsComplete,
						Lag:              plan.Lag,
					})
				}
			}
		}
	}

	return candidates
}

func resolveSelectorsForSymbol(
	selector Selector,
	symbol string,
	peer string,
	epoch uint64,
	store *ObservationStore,
) []Coordinate {
	matches := make([]Coordinate, 0)

	store.RangeCoordinatesForSymbol(symbol, func(coordinate Coordinate) bool {
		if coordinate.Epoch != epoch {
			return true
		}

		if peer != "" && coordinate.Peer != peer {
			return true
		}

		if !selector.Matches(coordinate) {
			return true
		}

		matches = append(matches, coordinate)
		return true
	})

	return matches
}
