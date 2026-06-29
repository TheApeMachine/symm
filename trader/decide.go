package trader

import (
	"math"
	"sort"
	"strings"

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
efe scores one candidate entry by Expected Free Energy. The playbook confidence
is the base candidate value, causal uplift is the optional pragmatic edge when
that model is identifiable, and manifold/resonance are precision terms for how
much to trust this symbol's current state.
*/
type efe struct {
	confidence float64
	surprise   float64
	uplift     float64
	hasUplift  bool
	field      field
	hasField   bool
}

/*
fieldPrecision maps the manifold field readout to a bounded trust weight. The
unit prior keeps uncalibrated/deferred field infrastructure from zeroing a core
playbook candidate, while large rupture/viscosity still down-weights it against
cleaner symbols.
*/
func (score efe) fieldPrecision() float64 {
	if !score.hasField {
		return 1
	}

	pragmatic := math.Max(0, score.field.coherence*score.field.guidance)
	risk := math.Max(0, score.field.viscosity*(1-score.field.coherence)+score.field.pressure)

	return (1 + pragmatic) / (1 + pragmatic + risk)
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
value is the negative expected free energy: the playbook confidence is the core
candidate value, causal counterfactual uplift scales it only when the causal
model is identifiable, and manifold/resonance refine it as precision signals.
Missing or not-yet-identifiable optional evidence is not a veto because the
playbook candidate already carries measured signal confidence.
*/
func (score efe) value() float64 {
	// The playbook's entry confidence is a required precision. Without it the
	// entry is unpriced — not full-precision. Absent confidence scores zero.
	if score.confidence <= 0 {
		return 0
	}

	value := score.confidence

	if score.hasUplift {
		// A ready causal model with no positive counterfactual edge has priced
		// the candidate as non-causal noise. Missing or unready causal evidence
		// is handled by hasUplift=false and stays non-blocking.
		if score.uplift <= 0 {
			return 0
		}

		value *= 1 + score.uplift/(1+score.uplift)
	}

	if score.hasField {
		value *= score.fieldPrecision()
	}

	return value * score.precision()
}

/*
Decider chooses which candidate actions reach the desk. Protective exits always
pass; entries are ranked by Expected Free Energy against the manifold field and
only those whose pragmatic edge beats their risk are dispatched.
*/
type Decider struct {
}

/*
NewDecider instantiates a Decider with the risk policy (slots and sizing) read
from config. The manifold field, surprises, and uplifts that drive ranking
arrive fresh in the measurement batch every tick.
*/
func NewDecider() *Decider {
	return &Decider{}
}

/*
verdict records how one candidate entry priced at the decision point. The reason
and score are carried back to the caller (and the UI) so an admitted or vetoed
entry is an observable, explained event — never a silent drop. "Honest failure"
requires the funnel to report which data source was missing or which gate fired,
not to vanish.
*/
type verdict struct {
	action *datura.Artifact
	symbol string
	reason string
	score  float64
}

/*
choose ranks the candidate actions and returns the subset to dispatch alongside
per-entry verdict metadata. Exits are passed through untouched; entries are
scored by expected free energy against the per-symbol manifold field, gated to
positive edge, and ordered best-first. Every entry comes back with the trader
score and reason — incomplete data sources surface as a recorded cause, not a
missing trade.
*/
func (decider *Decider) choose(
	measurements []*datura.Artifact,
	actions []*datura.Artifact,
	balances *datura.Artifact,
) ([]*datura.Artifact, []verdict) {
	if len(actions) == 0 {
		return actions, nil
	}

	fields := decider.fields(measurements)
	surprises := decider.surprises(measurements)
	uplifts := decider.uplifts(measurements)

	chosen := make([]*datura.Artifact, 0, len(actions))
	entries := make([]*datura.Artifact, 0, len(actions))
	verdicts := make([]verdict, 0, len(actions))

	for _, action := range actions {
		if isExit(action) {
			chosen = append(chosen, action)
			continue
		}

		symbol, _ := action.Scope()

		base, _, _ := strings.Cut(symbol, "/")
		held := false
		for index := range datura.Peek[[]any](balances, "data") {
			asset := datura.Peek[string](balances, "data", index, "asset")
			balance := datura.Peek[float64](balances, "data", index, "balance")

			if strings.EqualFold(asset, symbol) || strings.EqualFold(asset, base) {
				held = balance > 0
				break
			}
		}

		if held {
			stampVerdict(action, "blocked", "held", 0)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: "held",
			})
			continue
		}

		confidence := datura.Peek[float64](action, "entry_confidence")
		if confidence <= 0 {
			stampVerdict(action, "blocked", "no entry confidence", 0)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: "no entry confidence",
			})
			continue
		}

		readout, hasField := fields[symbol]
		upliftVal, hasCausal := uplifts[symbol]

		energy := efe{
			confidence: confidence,
			surprise:   surprises[symbol],
			uplift:     upliftVal,
			hasUplift:  hasCausal,
			field:      readout,
			hasField:   hasField,
		}
		score := energy.value()

		// A non-finite score must never pass the gate: NaN compares false to
		// every bound, so it would otherwise slip through and corrupt the sort.
		// Risk meeting or beating the edge raises free energy — also rejected.
		if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 {
			reason := "below edge"

			stampVerdict(action, "blocked", reason, score)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: reason,
			})
			continue
		}

		action.WithAttribute("decision.score", score)
		action.WithAttribute("decision.confidence", confidence)
		entries = append(entries, action)
	}

	sort.SliceStable(entries, func(first, second int) bool {
		firstScore := datura.Peek[float64](entries[first], "decision", "score")
		secondScore := datura.Peek[float64](entries[second], "decision", "score")

		return firstScore > secondScore
	})

	for _, action := range entries {
		chosen = append(chosen, action)
		symbol, _ := action.Scope()
		stampVerdict(action, "allow", "admitted", datura.Peek[float64](action, "decision", "score"))
		verdicts = append(verdicts, verdict{
			action: action,
			symbol: symbol,
			reason: "admitted",
			score:  datura.Peek[float64](action, "decision", "score"),
		})
	}

	return chosen, verdicts
}

func stampVerdict(
	action *datura.Artifact,
	verdict string,
	reason string,
	score float64,
) {
	if action == nil {
		return
	}

	confidence := datura.Peek[float64](action, "entry_confidence")

	action.WithAttribute(
		"source", "trader",
	).WithAttribute(
		"verdict", verdict,
	).WithAttribute(
		"why", reason,
	).WithAttribute(
		"score", score,
	).WithAttribute(
		"confidence", confidence,
	).WithAttribute(
		"decision.verdict", verdict,
	).WithAttribute(
		"decision.reason", reason,
	).WithAttribute(
		"decision.score", score,
	).WithAttribute(
		"decision.confidence", confidence,
	).WithAttribute(
		"journey.trader.decision", verdict,
	).WithAttribute(
		"journey.trader.reason", reason,
	)
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

		if !datura.Peek[bool](measurement, "output", "counterfactualReady") {
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
