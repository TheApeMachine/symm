package opportunity

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
armedPrecursorCount is the minimum number of distinct precursor systems that
must agree before a candidate advances from Forming to Armed. It is a declared
policy constant, not a tuned threshold: "multiple precursors agree" is the
definition of Armed.
*/
const armedPrecursorCount = 2

/*
family declares one opportunity archetype from the existing Category
vocabulary. Precursors are the present-tense states that historically precede
the transition; Confirmation is the category whose appearance marks the visible
ignition. It is data, not code, so the Category→archetype mapping stays
auditable in one place.
*/
type family struct {
	Archetype    types.OpportunityArchetype
	Direction    types.OpportunityDirection
	Precursors   []types.CategoryType
	Confirmation types.CategoryType
}

var families = []family{
	{
		Archetype: types.ArchetypeVerticalIgnition,
		Direction: types.DirectionLong,
		Precursors: []types.CategoryType{
			types.CoiledCompression,
			types.HiddenAbsorption,
			types.BookThinning,
			types.Frenzy,
			types.AdverseLeverageBuildup,
			types.InefficientLag,
		},
		Confirmation: types.VerticalIgnition,
	},
}

/*
resident is one symbol's tracked opportunity slot per archetype. It retains the
current candidate plus whether the hypothesis already resolved to invalidated,
so a dissolved precursor state emits Invalidated exactly once.
*/
type resident struct {
	candidate   types.OpportunityCandidate
	invalidated bool
}

/*
Solver synthesizes typed opportunity hypotheses from existing semantic outputs.
It is not a signal stage: it never re-derives measurements. It consumes the same
ranked category batch the cognition stage reads and tracks, per symbol and
archetype, whether a precursor state is forming, arming, or igniting.

Delivery is serialized per subscriber (ObservationalFIFO), so resident slots are
touched one symbol at a time and need no per-symbol lock.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	states        sync.Map // symbol -> map[OpportunityArchetype]*resident
	ObserveModule func(string, time.Duration)
}

/*
NewSolver constructs the synthesizer and, when a workspace is supplied, wires it
on ChannelCategories → ChannelOpportunities.
*/
func NewSolver(ctx context.Context, bus *runtime.Workspace) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
	}

	if bus != nil {
		runtime.Register[[]types.Category, []*types.OpportunityCandidate](
			bus,
			nil,
			solver.Step,
		)
	}

	return solver
}

func (solver *Solver) Name() string { return "opportunity" }

func (solver *Solver) Error() error { return nil }

/*
Close cancels the solver context.
*/
func (solver *Solver) Close() error {
	solver.cancel()
	return nil
}

/*
Step folds one ranked category batch into the opportunity tracker and returns
the updated candidates for that symbol. A batch with no active precursors or
confirmation returns nil: computation follows opportunity density, so dormant
symbols cost nothing.
*/
func (solver *Solver) Step(categories []types.Category) []*types.OpportunityCandidate {
	if len(categories) == 0 {
		return nil
	}

	symbol := categories[0].Symbol
	active := activeCategories(categories)
	slots := solver.slotsFor(symbol)
	now := time.Now().UTC()

	candidates := make([]*types.OpportunityCandidate, 0, len(families))

	for _, declared := range families {
		phase, found := phaseFor(declared, active)

		if candidate := solver.advance(slots, declared, symbol, phase, found, active, now); candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	return candidates
}

/*
slotsFor returns the resident opportunity slots for one symbol, creating an
empty table on first sight.
*/
func (solver *Solver) slotsFor(symbol string) map[types.OpportunityArchetype]*resident {
	loaded, _ := solver.states.LoadOrStore(symbol, map[types.OpportunityArchetype]*resident{})

	return loaded.(map[types.OpportunityArchetype]*resident)
}

/*
advance transitions one family's resident candidate through its lifecycle and
returns the candidate to publish, or nil when nothing changed (dormant, or an
already-emitted invalidation).
*/
func (solver *Solver) advance(
	slots map[types.OpportunityArchetype]*resident,
	declared family,
	symbol string,
	phase types.OpportunityPhase,
	found bool,
	active map[types.CategoryType]float64,
	now time.Time,
) *types.OpportunityCandidate {
	slot, exists := slots[declared.Archetype]

	if !found {
		if !exists {
			return nil
		}

		if slot.invalidated {
			return nil
		}

		slot.invalidated = true
		slot.candidate.Phase = types.PhaseInvalidated
		slot.candidate.Updated = now
		slot.candidate.Sequence++
		delete(slots, declared.Archetype)

		return &slot.candidate
	}

	maturity := familyMaturity(declared, active)

	if !exists {
		candidate := types.OpportunityCandidate{
			Symbol:     symbol,
			Archetype:  declared.Archetype,
			Phase:      phase,
			Direction:  declared.Direction,
			FirstSeen:  now,
			Updated:    now,
			Sequence:   1,
			Provenance: types.ProvenanceCategory,
			Maturity:   maturity,
		}

		slots[declared.Archetype] = &resident{candidate: candidate}

		return &candidate
	}

	slot.candidate.Phase = phase
	slot.candidate.Updated = now
	slot.candidate.Sequence++
	slot.candidate.Provenance |= types.ProvenanceCategory
	slot.candidate.Maturity = maturity

	return &slot.candidate
}

/*
activeCategories reduces a ranked batch to the categories carrying positive
support, keyed by category and valued by maturity. Strength > 0 is the honest
"this category has supporting evidence" signal; Confidence is an evidence share
that stays near-uniform when many regimes fire.
*/
func activeCategories(categories []types.Category) map[types.CategoryType]float64 {
	active := make(map[types.CategoryType]float64, len(categories))

	for _, category := range categories {
		if category.Strength > 0 {
			active[category.Type] = category.Maturity
		}
	}

	return active
}

/*
phaseFor resolves one family's lifecycle phase from the currently active
categories. Confirmation dominates: once the visible transition begins the
hypothesis is Ignition regardless of how many precursors remain.
*/
func phaseFor(declared family, active map[types.CategoryType]float64) (types.OpportunityPhase, bool) {
	if _, ignited := active[declared.Confirmation]; ignited {
		return types.PhaseIgnition, true
	}

	count := 0

	for _, precursor := range declared.Precursors {
		if _, present := active[precursor]; present {
			count++
		}
	}

	if count >= armedPrecursorCount {
		return types.PhaseArmed, true
	}

	if count >= 1 {
		return types.PhaseForming, true
	}

	return types.PhaseDormant, false
}

/*
familyMaturity is the strongest maturity among the family's active evidence,
without allocating a combined slice.
*/
func familyMaturity(declared family, active map[types.CategoryType]float64) float64 {
	maturity := 0.0

	for _, precursor := range declared.Precursors {
		if value, present := active[precursor]; present && value > maturity {
			maturity = value
		}
	}

	if value, present := active[declared.Confirmation]; present && value > maturity {
		maturity = value
	}

	return maturity
}
