package relation

import (
	"errors"
	"iter"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

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
selectorMatches reports whether a coordinate satisfies the selector. An empty
field is a wildcard for that identity component.
*/
func selectorMatches(selector Selector, coordinate Coordinate) bool {
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
CompiledCandidate represents a pre-resolved (Source, Target, Controls, Lag)
candidate pair.
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
CompileRequest asks the planner to precompile the relation candidates across
all active plans for one symbol. Coordinates are the symbol's resident
coordinates in canonical order; the planner resolves selectors against them
structurally, never against evidence.
*/
type CompileRequest struct {
	Plans       []*RelationPlan
	Symbol      string
	Epoch       uint64
	Coordinates []Coordinate
}

/*
CompileResult is the compiled candidates of one request.
*/
type CompileResult struct {
	Candidates []CompiledCandidate
}

/*
Planner compiles RelationPlans into explicit candidates. Eligibility is
structural only: symbol scope, peer scope, explicit pairs, and exact
controls, resolved against the resident coordinates supplied per request.
*/
type Planner struct {
	err error
	out CompileResult
}

/*
NewPlanner creates a Planner primitive.
*/
func NewPlanner() core.Primitive {
	return &Planner{}
}

/*
Next receives *CompileRequest payloads and yields a *CompileResult with the
compiled candidates for each.
*/
func (op *Planner) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			request := (*CompileRequest)(arriving)
			op.out = CompileResult{
				Candidates: compileCandidates(
					request.Plans, request.Symbol, request.Epoch, request.Coordinates,
				),
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Planner) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
compileCandidates precompiles the relation candidates across all active plans
for one symbol against the symbol's resident coordinates.
*/
func compileCandidates(
	plans []*RelationPlan,
	symbol string,
	epoch uint64,
	coordinates []Coordinate,
) []CompiledCandidate {
	var candidates []CompiledCandidate

	for _, plan := range plans {
		if plan == nil || plan.Epoch != epoch {
			continue
		}

		controls, controlsComplete := resolveControls(plan, coordinates)

		for _, pair := range pairsForSymbol(plan, symbol) {
			sources := resolveSelector(pair.Source, coordinates, plan.Peer, epoch)
			targets := resolveSelector(pair.Target, coordinates, plan.Peer, epoch)

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

/*
pairsForSymbol returns the planned pairs applicable to one symbol, or nil
when the plan's scope excludes it. Cross-product pairs are expanded
structurally; self-pairs (identical Source and Target selectors) are excluded
because Influence requires a positive lag between distinct coordinates.
*/
func pairsForSymbol(plan *RelationPlan, symbol string) []PlannedPair {
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
resolveControls resolves the plan's control selectors against the resident
coordinates available for the symbol, returning explicit controls in selector
order. The boolean reports whether every exact control selector resolved to
a resident coordinate. A wildcard selector resolves to every matching
coordinate; this is structural availability, not evidence. A missing exact
control makes the Relation unavailable rather than silently changing the
model.
*/
func resolveControls(plan *RelationPlan, coordinates []Coordinate) ([]Control, bool) {
	controls := make([]Control, 0, len(plan.Controls))

	for _, selector := range plan.Controls {
		matched := false

		for _, coordinate := range coordinates {
			if plan.Peer != "" && coordinate.Peer != plan.Peer {
				continue
			}

			if !selectorMatches(selector.Selector, coordinate) {
				continue
			}

			controls = append(controls, Control{Coordinate: coordinate, Lag: selector.Lag})
			matched = true
		}

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
resolveSelector resolves one selector against the symbol's resident
coordinates, filtered by model epoch and peer scope.
*/
func resolveSelector(
	selector Selector,
	coordinates []Coordinate,
	peer string,
	epoch uint64,
) []Coordinate {
	matches := make([]Coordinate, 0)

	for _, coordinate := range coordinates {
		if coordinate.Epoch != epoch {
			continue
		}

		if peer != "" && coordinate.Peer != peer {
			continue
		}

		if !selectorMatches(selector, coordinate) {
			continue
		}

		matches = append(matches, coordinate)
	}

	return matches
}
