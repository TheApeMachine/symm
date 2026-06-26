package trader

import (
	"math"
	"sort"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

/*
field is the manifold's per-symbol generative-model readout for one tick: the
coherence, guidance, viscosity, and pressure-gradient the GPU fluid solver
published on its measurement artifact (signal/manifold/signal.go). It is the
agent's forward model of the market state at this symbol's location in the field.
*/
type field struct {
	coherence float64
	guidance  float64
	viscosity float64
	pressure  float64
}

/*
efe scores one candidate entry by Expected Free Energy, faithful to the cascade:
the causal counterfactual uplift (signal/causal abductive do(flow)) is the
pragmatic value — the policy's predicted edge from acting. The manifold field
(organization vs rupture) and the resonance reconstruction (inverse free energy)
are precisions: how much to trust that counterfactual for this symbol. The
playbook's entry confidence is a third precision. Minimizing expected free
energy is maximizing value().
*/
type efe struct {
	confidence float64
	surprise   float64
	uplift     float64
	field      field
}

/*
fieldEdge is the manifold's signed verdict: organized, directional energy
(coherence × guidance) minus incoherence and rupture (viscosity decoupling plus
pressure-gradient shock). Positive means the field is a clean wave; negative
means a turbulent, ruptured cell the field vetoes regardless of the causal call.
*/
func (score efe) fieldEdge() float64 {
	pragmatic := score.field.coherence * score.field.guidance
	risk := score.field.viscosity*(1-score.field.coherence) + score.field.pressure

	return pragmatic - risk
}

/*
precision maps the resonance model's reconstruction surprise (free
energy, ≥ 0) to a bounded precision in (0, 1], mirroring predictive coding's
precision = 1/(variance) (signal resonance bprecision_update). A symbol the
model reconstructs perfectly (surprise 0) — or one resonance has no data for —
weights at unit precision; the worse the reconstruction, the less we trust the
field's forward roll for that symbol.
*/
func (score efe) precision() float64 {
	return 1 / (1 + math.Max(0, score.surprise))
}

/*
value is the negative expected free energy: the causal counterfactual uplift
weighted by the playbook, manifold, and resonance precisions. Positive means the
counterfactual predicts a real, organized, trustworthy edge from acting; zero or
below means there is no causal edge or the field cannot vouch for it.

value never substitutes a default for missing evidence: a non-positive
confidence, uplift, or field edge yields zero. The caller treats a zero score as
"could not price this entry" and records WHY (which precision was absent) rather
than fabricating an identity weight that would let an unpriced entry trade.
*/
func (score efe) value() float64 {
	// The playbook's entry confidence is a required precision. Without it the
	// entry is unpriced — not full-precision. Absent confidence scores zero.
	if score.confidence <= 0 {
		return 0
	}

	// The counterfactual must predict a gain from acting; without a positive
	// causal edge there is nothing to act on.
	if score.uplift <= 0 {
		return 0
	}

	// The field vetoes incoherent, ruptured states even when the counterfactual
	// is positive — and guards against a negative×negative sign flip.
	edge := score.fieldEdge()

	if edge <= 0 {
		return 0
	}

	return score.confidence * score.precision() * score.uplift * edge
}

/*
Decider chooses which candidate actions reach the desk. Protective exits always
pass; entries are ranked by Expected Free Energy against the manifold field and
only those whose pragmatic edge beats their risk are dispatched.
*/
type Decider struct {
	alloc allocation
}

/*
NewDecider instantiates a Decider with the risk policy (slots and sizing) read
from config. The manifold field, surprises, and uplifts that drive ranking
arrive fresh in the measurement batch every tick.
*/
func NewDecider() *Decider {
	return &Decider{alloc: newAllocation()}
}

/*
verdict records why one candidate entry did not reach the desk. The reason is
carried back to the caller (and the UI) so a vetoed entry is an observable,
explained event — never a silent drop. "Honest failure" requires the funnel to
report which data source was missing or which gate fired, not to vanish.
*/
type verdict struct {
	action *datura.Artifact
	symbol string
	reason string
}

/*
choose ranks the candidate actions and returns the subset to dispatch alongside
the explained rejections. Exits are passed through untouched; entries are scored
by expected free energy against the per-symbol manifold field, gated to positive
edge, and ordered best-first. Every entry that is NOT dispatched comes back as a
verdict with its reason — incomplete data sources surface as a recorded cause,
not a missing trade.
*/
func (decider *Decider) choose(
	measurements []*datura.Artifact,
	actions []*datura.Artifact,
	balances *logic.Balances,
) ([]*datura.Artifact, []verdict) {
	if len(actions) == 0 {
		return actions, nil
	}

	fields := decider.fields(measurements)
	surprises := decider.surprises(measurements)
	uplifts := decider.uplifts(measurements)

	chosen := make([]*datura.Artifact, 0, len(actions))
	entries := make([]rankedEntry, 0, len(actions))
	rejected := make([]verdict, 0, len(actions))

	for _, action := range actions {
		if isExit(action) {
			chosen = append(chosen, action)
			continue
		}

		symbol, _ := action.Scope()

		// Already holding this symbol: do not stack a fresh entry on an open
		// position. The ledger, not the field, vetoes here.
		if balances.Held(symbol) {
			rejected = append(rejected, verdict{action, symbol, "held"})
			continue
		}

		// Every precision the score needs must be measured for this symbol — the
		// field cannot price a symbol it never observed, and the counterfactual
		// cannot vouch for a symbol it never traded. A missing source is an
		// honest, recorded rejection, never a fabricated neutral default that
		// would let an unpriced entry through.
		readout, known := fields[symbol]
		if !known {
			rejected = append(rejected, verdict{action, symbol, "no manifold field for symbol"})
			continue
		}

		confidence := datura.Peek[float64](action, "entry_confidence")
		if confidence <= 0 {
			rejected = append(rejected, verdict{action, symbol, "no entry confidence"})
			continue
		}

		upliftVal, hasCausal := uplifts[symbol]
		if !hasCausal {
			rejected = append(rejected, verdict{action, symbol, "no causal uplift for symbol"})
			continue
		}

		score := efe{
			confidence: confidence,
			surprise:   surprises[symbol],
			uplift:     upliftVal,
			field:      readout,
		}.value()

		// A non-finite score must never pass the gate: NaN compares false to
		// every bound, so it would otherwise slip through and corrupt the sort.
		// Risk meeting or beating the edge raises free energy — also rejected.
		if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 {
			rejected = append(rejected, verdict{action, symbol, "below edge"})
			continue
		}

		entries = append(entries, rankedEntry{
			action:     action,
			score:      score,
			confidence: confidence,
		})
	}

	sort.SliceStable(entries, func(first, second int) bool {
		return entries[first].score > entries[second].score
	})

	admitted := decider.alloc.admit(entries, balances)

	admittedSet := make(map[*datura.Artifact]struct{}, len(admitted))
	for _, entry := range admitted {
		chosen = append(chosen, entry.action)
		admittedSet[entry.action] = struct{}{}
	}

	// Entries that scored positive but lost the slot contest are still
	// rejections the UI must explain — no slot is a reason, not a silent drop.
	for _, entry := range entries {
		if _, ok := admittedSet[entry.action]; !ok {
			symbol, _ := entry.action.Scope()
			rejected = append(rejected, verdict{entry.action, symbol, "no slot"})
		}
	}

	return chosen, rejected
}

/*
fields indexes the manifold measurements in this batch by symbol, exposing the
field readout the solver published for each.
*/
func (decider *Decider) fields(
	measurements []*datura.Artifact,
) map[string]field {
	fields := make(map[string]field)

	for _, measurement := range measurements {
		symbol, ok := scopeForOrigin(measurement, logic.SourceManifold)

		if !ok {
			continue
		}

		fields[symbol] = field{
			coherence: datura.Peek[float64](measurement, "output", "coherenceMag2"),
			guidance:  datura.Peek[float64](measurement, "output", "guidanceSpeed"),
			viscosity: datura.Peek[float64](measurement, "output", "viscosityProxy"),
			pressure:  datura.Peek[float64](measurement, "output", "pressureGradNorm"),
		}
	}

	return fields
}

/*
surprises indexes the resonance measurements in this batch by symbol, exposing
the reconstruction free energy the resonance model published for each. Symbols
without a resonance measurement are simply absent (precision defaults to unit).
*/
func (decider *Decider) surprises(
	measurements []*datura.Artifact,
) map[string]float64 {
	surprises := make(map[string]float64)

	for _, measurement := range measurements {
		symbol, ok := scopeForOrigin(measurement, logic.SourceResonance)

		if !ok {
			continue
		}

		surprises[symbol] = datura.Peek[float64](measurement, "surprise")
	}

	return surprises
}

/*
uplifts indexes the causal measurements in this batch by symbol, exposing the
signed counterfactual uplift (do(flow) on the symbol's structural model) the
causal signal published for each.
*/
func (decider *Decider) uplifts(
	measurements []*datura.Artifact,
) map[string]float64 {
	uplifts := make(map[string]float64)

	for _, measurement := range measurements {
		symbol, ok := scopeForOrigin(measurement, logic.SourceCausal)

		if !ok {
			continue
		}

		uplifts[symbol] = datura.Peek[float64](measurement, "output", "uplift")
	}

	return uplifts
}

/*
scopeForOrigin returns the measurement's scope (symbol) when its origin matches
want, reporting ok=false otherwise so callers can skip foreign or malformed
measurements without a silent default.
*/
func scopeForOrigin(
	measurement *datura.Artifact,
	want logic.SourceType,
) (string, bool) {
	origin := errnie.Does(func() (string, error) {
		return measurement.Origin()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"decider: failed to get measurement origin",
			err,
		))
	}).Value()

	if origin != string(want) {
		return "", false
	}

	symbol := errnie.Does(func() (string, error) {
		return measurement.Scope()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"decider: failed to get measurement scope",
			err,
		))
	}).Value()

	if symbol == "" {
		return "", false
	}

	return symbol, true
}

/*
isExit reports whether the candidate is a protective exit, which is never
throttled by ranking.
*/
func isExit(action *datura.Artifact) bool {
	return logic.ActionType(datura.Peek[string](action, "type")).IsExit()
}
