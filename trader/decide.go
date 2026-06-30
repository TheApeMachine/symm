package trader

import (
	"math"
	"sort"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
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

	if score.hasUplift && score.uplift > 0 {
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
	economics executionEconomics
	tree      *dmt.Tree
}

/*
NewDecider instantiates a Decider with the risk policy (slots and sizing) read
from config. The manifold field, surprises, and uplifts that drive ranking
arrive fresh in the measurement batch every tick.
*/
func NewDecider(trees ...*dmt.Tree) *Decider {
	var tree *dmt.Tree
	if len(trees) > 0 {
		tree = trees[0]
	}

	return &Decider{economics: newExecutionEconomics(), tree: tree}
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

		if holdsSymbol(balanceRows(balances), symbol) {
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
			stampVerdict(action, "blocked", "no_entry_confidence", 0)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: "no_entry_confidence",
			})
			continue
		}

		readout, hasField := fields[symbol]
		upliftVal, hasCausal := uplifts[symbol]
		economics := decider.economics.price(action, upliftVal, hasCausal, decider.tree)

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
			reason := "below_edge"

			stampEconomics(action, economics)
			stampVerdict(action, "blocked", reason, score)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: reason,
			})
			continue
		}

		stampEconomics(action, economics)

		if !economics.calibrationReady {
			reason := "edge_unavailable"

			stampVerdict(action, "blocked", reason, score)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: reason,
				score:  score,
			})
			continue
		}

		if economics.netEdgeBps < viperEdgeMinBps() {
			reason := "below_edge"

			stampVerdict(action, "blocked", reason, score)
			verdicts = append(verdicts, verdict{
				action: action,
				symbol: symbol,
				reason: reason,
				score:  score,
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

func stampEconomics(action *datura.Artifact, economics economicPrice) {
	if action == nil {
		return
	}

	action.WithAttribute(
		"edge_key", economics.edgeKey,
	).WithAttribute(
		"decision.edge_key", economics.edgeKey,
	).WithAttribute(
		"decision.edge", economics.edge,
	).WithAttribute(
		"decision.expected_return_bps", economics.expectedReturnBps,
	).WithAttribute(
		"decision.net_edge_bps", economics.netEdgeBps,
	).WithAttribute(
		"decision.sample_count", economics.sampleCount,
	).WithAttribute(
		"decision.calibration_ready", economics.calibrationReady,
	).WithAttribute(
		"decision.edge_source", economics.edgeSource,
	).WithAttribute(
		"decision.hurdle", economics.hurdle,
	).WithAttribute(
		"decision.friction", economics.hurdle,
	).WithAttribute(
		"decision.hurdle_bps", economics.hurdle*10_000,
	).WithAttribute(
		"decision.friction_bps", economics.hurdle*10_000,
	).WithAttribute(
		"decision.economic_priced", economics.priced,
	).WithAttribute(
		"execution.liquidity", economics.liquidity,
	)
}

func viperEdgeMinBps() float64 {
	edgeMin := viper.GetFloat64("trading.edge_min_bps")
	if edgeMin <= 0 {
		return 10
	}

	return edgeMin
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
surprises indexes the DMT cognitive surprise stamped onto the live measurements
by market.ApplyCognitiveReadings. Symbols without a cognitive reading are absent
(precision defaults to unit); the decider does not invent a fallback surprise.
*/
func (decider *Decider) surprises(
	measurements []*datura.Artifact,
) map[string]float64 {
	raw := make(map[string]float64)

	for _, measurement := range measurements {
		symbol, err := measurement.Scope()

		if err != nil || symbol == "" {
			continue
		}

		surprise := datura.Peek[float64](measurement, "output", "surprise")

		if surprise <= 0 {
			continue
		}

		if math.IsNaN(surprise) || math.IsInf(surprise, 0) {
			continue
		}

		if prior, ok := raw[symbol]; !ok || surprise > prior {
			raw[symbol] = surprise
		}
	}

	if len(raw) < 2 {
		return map[string]float64{}
	}

	samples := make([]float64, 0, len(raw))
	for _, surprise := range raw {
		samples = append(samples, surprise)
	}

	center := statutil.Median(samples)
	scale := statutil.MedianAbsoluteDeviation(samples, center)
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = finiteSpan(samples)
	}
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return map[string]float64{}
	}

	relative := make(map[string]float64, len(raw))
	for symbol, surprise := range raw {
		excess := (surprise - center) / scale
		if excess > 0 && !math.IsNaN(excess) && !math.IsInf(excess, 0) {
			relative[symbol] = excess
		}
	}

	return relative
}

func finiteSpan(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	minimum := math.Inf(1)
	maximum := math.Inf(-1)
	for _, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			continue
		}
		if sample < minimum {
			minimum = sample
		}
		if sample > maximum {
			maximum = sample
		}
	}

	if math.IsInf(minimum, 0) || math.IsInf(maximum, 0) {
		return 0
	}

	return maximum - minimum
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
