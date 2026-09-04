package opportunity

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
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

Each workload serializes its own delivery, but ticker, trade, and level-3
workloads share this solver. The lifecycle transition is therefore serialized
here so a candidate's phase and sequence remain one atomic state change.
*/
type Solver struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	status *runtime.Status
	mutex  sync.Mutex

	states        sync.Map // symbol -> map[OpportunityArchetype]*resident
	ObserveModule func(string, time.Duration)
}

/*
NewSolver constructs the opportunity synthesizer.
*/
func NewSolver(ctx context.Context) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	return &Solver{
		ctx:    ctx,
		cancel: cancel,
		status: runtime.NewStatus().Transition(runtime.READY),
	}
}

func (solver *Solver) Name() string { return "opportunity" }

func (solver *Solver) Error() error { return solver.err }

/*
Close cancels the solver context.
*/
func (solver *Solver) Close() error {
	solver.cancel()
	return nil
}

/*
Step folds this envelope's ranked category batch into the opportunity tracker
and writes the updated candidates back onto the envelope. An envelope with no
categories, or no active precursors/confirmation, leaves Opportunities unset:
computation follows opportunity density, so dormant symbols cost nothing.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if solver.err != nil {
		solver.cancel()

		return nil
	}

	categories := envelope.Categories

	if len(categories) == 0 {
		symbol := envelopeSymbol(envelope)

		if symbol == "" {
			return envelope
		}

		solver.mutex.Lock()
		defer solver.mutex.Unlock()

		slots := solver.slotsFor(symbol)
		eventTime := envelopeEventTime(envelope)

		candidates := make([]*types.OpportunityCandidate, 0, len(slots))

		if precursor := solver.checkVolumeSurgePrecursor(envelope, slots, symbol, eventTime); precursor != nil {
			candidates = append(candidates, precursor)
		}

		for _, resident := range slots {
			if resident == nil || resident.invalidated {
				continue
			}

			if resident.candidate.Phase == types.PhaseArmed || resident.candidate.Phase == types.PhaseIgnition {
				alreadyAdded := false

				for _, candidate := range candidates {
					if candidate.Archetype == resident.candidate.Archetype {
						alreadyAdded = true
						break
					}
				}

				if !alreadyAdded {
					candidateCopy := resident.candidate
					candidates = append(candidates, &candidateCopy)
				}
			}
		}

		if len(candidates) > 0 {
			envelope.Opportunities = candidates
		}

		return envelope
	}

	solver.mutex.Lock()
	defer solver.mutex.Unlock()

	symbol, eventTime, err := validateBatch(categories)

	if err != nil {
		solver.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"opportunity: invalid category batch",
			err,
		))
		solver.status.Transition(runtime.FATAL)
		solver.cancel()

		return envelope
	}

	active := activeCategories(categories)

	slots := solver.slotsFor(symbol)

	candidates := make([]*types.OpportunityCandidate, 0, len(families))

	for _, declared := range families {
		phase, found := phaseFor(declared, active)

		if phase == types.PhaseArmed && !hasIndependentPrecursors(declared, categories) {
			phase = types.PhaseForming
		}

		if candidate := solver.advance(slots, declared, symbol, phase, found, active, eventTime); candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	if precursor := solver.checkVolumeSurgePrecursor(envelope, slots, symbol, eventTime); precursor != nil {
		candidates = append(candidates, precursor)
	}

	if len(candidates) > 0 {
		envelope.Opportunities = candidates
	}

	return envelope
}

func hasIndependentPrecursors(declared family, categories []types.Category) bool {
	sources := make(map[string]bool)
	precursorCount := 0

	for _, category := range categories {
		if category.Strength < 0.5 || category.Maturity <= 0 {
			continue
		}

		isPrecursor := false

		for _, precursor := range declared.Precursors {
			if category.Type == precursor {
				isPrecursor = true
				break
			}
		}

		if isPrecursor {
			precursorCount++

			for _, src := range category.Supporting {
				sources[src] = true
			}
		}
	}

	if precursorCount < armedPrecursorCount {
		return false
	}

	if len(sources) > 0 && len(sources) < 2 {
		return false
	}

	return true
}

func (solver *Solver) checkVolumeSurgePrecursor(
	envelope *types.Envelope,
	slots map[types.OpportunityArchetype]*resident,
	symbol string,
	eventTime time.Time,
) *types.OpportunityCandidate {
	if envelope == nil || envelope.PumpDump == nil {
		return nil
	}

	ratioMetric, hasRatio := envelope.PumpDump.Metrics["notional_rate_ratio"]
	zscoreMetric, hasZScore := envelope.PumpDump.Metrics["notional_rate_zscore"]
	velMetric, hasVel := envelope.PumpDump.Metrics["notional_rate_velocity"]

	ratio := 0.0

	if hasRatio {
		ratio = ratioMetric.Raw
	}

	zscore := 0.0

	if hasZScore {
		zscore = zscoreMetric.Raw
	}

	velocity := 0.0

	if hasVel {
		velocity = velMetric.Raw
	}

	isSurge := ratio >= 100.0 || (zscore >= 3.0 && velocity > 0)

	archetype := types.ArchetypeVolumeSurgePrecursor
	slot, exists := slots[archetype]

	if !isSurge {
		if !exists || slot.invalidated {
			return nil
		}

		dumpMetric, hasDump := envelope.PumpDump.Metrics["is_dump"]
		isContradicted := hasDump && dumpMetric.Raw > 0

		if isContradicted {
			slot.invalidated = true
			slot.candidate.Phase = types.PhaseInvalidated
			slot.candidate.Updated = eventTime
			slot.candidate.Sequence++
			delete(slots, archetype)

			candidate := slot.candidate

			return &candidate
		}

		candidate := slot.candidate

		return &candidate
	}

	maturity := 0.5

	if zscore >= 3.0 {
		maturity = math.Min(1.0, zscore/6.0)
	} else if ratio >= 100.0 {
		maturity = math.Min(1.0, ratio/200.0)
	}

	if !exists {
		candidate := types.OpportunityCandidate{
			Symbol:     symbol,
			Archetype:  archetype,
			Phase:      types.PhaseArmed,
			Direction:  types.DirectionLong,
			FirstSeen:  eventTime,
			Updated:    eventTime,
			Sequence:   1,
			Provenance: types.ProvenanceCognition,
			Maturity:   maturity,
		}
		slots[archetype] = &resident{candidate: candidate}

		return &candidate
	}

	if eventTime.Before(slot.candidate.Updated) {
		candidate := slot.candidate

		return &candidate
	}

	slot.candidate.Phase = types.PhaseArmed
	slot.candidate.Updated = eventTime
	slot.candidate.Sequence++
	slot.candidate.Provenance = types.ProvenanceCognition
	slot.candidate.Maturity = maturity

	candidate := slot.candidate

	return &candidate
}

func validateBatch(categories []types.Category) (string, time.Time, error) {
	if len(categories) == 0 {
		return "", time.Time{}, nil
	}

	symbol := categories[0].Symbol
	eventTime := categories[0].At

	if symbol == "" || eventTime.IsZero() {
		return "", time.Time{}, fmt.Errorf(
			"category symbol and event time required",
		)
	}

	for index := 1; index < len(categories); index++ {
		if categories[index].Symbol != symbol || !categories[index].At.Equal(eventTime) {
			return "", time.Time{}, fmt.Errorf(
				"category batch requires one symbol and event time",
			)
		}
	}

	return symbol, eventTime, nil
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
	eventTime time.Time,
) *types.OpportunityCandidate {
	slot, exists := slots[declared.Archetype]

	if exists && eventTime.Before(slot.candidate.Updated) {
		candidate := slot.candidate

		return &candidate
	}

	if !found {
		if !exists {
			return nil
		}

		if slot.invalidated {
			return nil
		}

		slot.invalidated = true
		slot.candidate.Phase = types.PhaseInvalidated
		slot.candidate.Updated = eventTime
		slot.candidate.Sequence++
		delete(slots, declared.Archetype)

		candidate := slot.candidate

		return &candidate
	}

	maturity := familyMaturity(declared, active)

	if !exists {
		candidate := types.OpportunityCandidate{
			Symbol:     symbol,
			Archetype:  declared.Archetype,
			Phase:      phase,
			Direction:  declared.Direction,
			FirstSeen:  eventTime,
			Updated:    eventTime,
			Sequence:   1,
			Provenance: types.ProvenanceCategory,
			Maturity:   maturity,
		}

		slots[declared.Archetype] = &resident{candidate: candidate}

		return &candidate
	}

	slot.candidate.Phase = phase
	slot.candidate.Updated = eventTime
	slot.candidate.Sequence++
	slot.candidate.Provenance |= types.ProvenanceCategory
	slot.candidate.Maturity = maturity

	candidate := slot.candidate

	return &candidate
}

/*
activeCategories reduces a ranked batch to the categories carrying material
support. Arming requires material strength and positive maturity.
*/
func activeCategories(categories []types.Category) map[types.CategoryType]float64 {
	active := make(map[types.CategoryType]float64, len(categories))

	for _, category := range categories {
		if category.Strength >= 0.5 && category.Maturity > 0 {
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
familyMaturity computes conjunction semantics: the minimum maturity among
active required precursors, reflecting the weakest link of the hypothesis.
*/
func familyMaturity(declared family, active map[types.CategoryType]float64) float64 {
	if val, confirmed := active[declared.Confirmation]; confirmed {
		return val
	}

	minMaturity := 1.0
	count := 0

	for _, precursor := range declared.Precursors {
		if value, present := active[precursor]; present {
			count++

			if value < minMaturity {
				minMaturity = value
			}
		}
	}

	if count < armedPrecursorCount {
		if count > 0 {
			return minMaturity
		}

		return 0.0
	}

	return minMaturity
}

func envelopeSymbol(envelope *types.Envelope) string {
	if envelope == nil {
		return ""
	}

	if envelope.TickerData.Symbol != "" {
		return envelope.TickerData.Symbol
	}

	if envelope.TradeData.Symbol != "" {
		return envelope.TradeData.Symbol
	}

	if envelope.PumpDump != nil && envelope.PumpDump.Symbol() != "" {
		return envelope.PumpDump.Symbol()
	}

	if envelope.Key != "" {
		return envelope.Key
	}

	return ""
}

func envelopeEventTime(envelope *types.Envelope) time.Time {
	if envelope == nil {
		return time.Time{}
	}

	if !envelope.TickerData.Timestamp.IsZero() {
		return envelope.TickerData.Timestamp
	}

	if !envelope.TradeData.Timestamp.IsZero() {
		return envelope.TradeData.Timestamp
	}

	if envelope.PumpDump != nil && !envelope.PumpDump.At.IsZero() {
		return envelope.PumpDump.At
	}

	return time.Now()
}
