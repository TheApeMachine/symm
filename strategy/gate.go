package strategy

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/system"
)

/*
chordGate is the chord-level surprise gate of one command: one resonance
solver (pace resonance.learning_rate, a predictive coder per symbol) and the
settings it runs by. The midpoint rule proposes a chord (symbolTape.advance);
the chord is appended only once the symbol is warm (its coder's error has
settled, see settling) and its surprise z-score since then reaches
SurpriseZ. With the gate off there is no solver and every proposed chord is
appended.
*/
type chordGate struct {
	settings data.PolicyGate
	solver   *resonance.Solver
}

/*
newChordGate validates cfg and builds the gate it describes.
*/
func newChordGate(ctx context.Context, cfg *system.Resonance) (*chordGate, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errnie.Err(errnie.Validation, "[gate] resonance configuration", err)
	}

	gate := &chordGate{settings: gateSettings(cfg)}

	if !cfg.Gate {
		return gate, nil
	}

	solver, err := resonance.NewSolver(ctx, cfg.LearningRate)

	if err != nil {
		return nil, err
	}

	gate.solver = solver
	return gate, nil
}

/*
gateSettings are the PolicyGate settings of cfg (no epochs): the settled
warm-up at the coders' own fast and slow rates.
*/
func gateSettings(cfg *system.Resonance) data.PolicyGate {
	fast, slow := resonance.Rates(cfg.LearningRate)

	return data.PolicyGate{
		Enabled: cfg.Gate, SurpriseZ: cfg.SurpriseZ, LearningRate: cfg.LearningRate,
		Warmup: data.PolicyWarmup{Rule: data.WarmupSettled, FastRate: fast, SlowRate: slow},
	}
}

/*
Why a symbol is still cold (GateReading.Cold, the trainer's funnel).
*/
const (
	// coldSilent: the coder has reported no error yet (an exact 0 before
	// its first non-zero error: its input standardizers are not ready, so it
	// has nothing to reconstruct). Not counted as history.
	coldSilent = "no error yet"
	// coldHistory: the slow error average has not yet seen one of its own
	// time constants (1/slow errors), so comparing against it says nothing.
	coldHistory = "history"
	// coldFalling: the coder's error is still falling, its fast average
	// below its slow one.
	coldFalling = "falling"
)

/*
GateReading is what the gate made of one symbol's tick: the solver's
surprise, whether the symbol is warm (and, if not, why: Cold), its z-score
against the symbol's surprises since it warmed (null until two exist), and
whether a chord proposed on this tick may be appended.
*/
type GateReading struct {
	Surprise float64  `json:"surprise"`
	Z        *float64 `json:"z"`
	Warm     bool     `json:"warm"`
	Cold     string   `json:"cold,omitempty"`
	Passed   bool     `json:"passed"`
}

/*
settling decides when a symbol's coder has settled. It averages the coder's
error (its surprise) exponentially at the two rates of the coder's Pace: fast
(the fastest it learns) and slow (the pace it rests at), from its first
non-zero error on. While the coder is still learning the error falls, and
the fast average, following it more closely, sits below the slow one. Once
the slow average has seen one of its own time constants (1/slow errors) and
the fast one is no longer below it, the error has stopped falling (within
its noise): the symbol is warm, and stays warm.
*/
type settling struct {
	fastRate float64
	slowRate float64
	steps    int
	fast     float64
	slow     float64
	warm     bool
}

/*
observe folds one error in and reports whether the symbol is warm, or why
it is still cold.
*/
func (settle *settling) observe(err float64) (bool, string) {
	if settle.steps == 0 && err == 0 {
		return false, coldSilent
	}

	settle.steps++

	if settle.steps == 1 {
		settle.fast, settle.slow = err, err
	} else {
		settle.fast += settle.fastRate * (err - settle.fast)
		settle.slow += settle.slowRate * (err - settle.slow)
	}

	if settle.warm {
		return true, ""
	}

	if float64(settle.steps) < math.Ceil(1/settle.slowRate) {
		return false, coldHistory
	}

	if settle.fast < settle.slow {
		return false, coldFalling
	}

	settle.warm = true
	return true, ""
}

/*
surpriseMoments are a symbol's warm-up state and, once warm, its running
surprise mean and sum of squared deviations (Welford) over every warm solver
step before the current one.
*/
type surpriseMoments struct {
	settle settling
	count  int
	mean   float64
	m2     float64
}

/*
read steps the solver on one symbol of tick and scores its surprise
causally: until the symbol is warm nothing passes and nothing accumulates;
once warm the z-score is taken against the moments before this surprise,
which is folded in afterwards. A nil reading means the gate is off.
*/
func (gate *chordGate) read(moments *surpriseMoments, tick *data.Value, symbol *broker.SymbolTick) (*GateReading, error) {
	if gate == nil || gate.solver == nil {
		return nil, nil
	}

	reading, err := gate.solver.Step(tick, symbol)

	if err != nil {
		return nil, err
	}

	surprise, err := resonance.ReadSurprise(reading)

	if err != nil {
		return nil, err
	}

	if math.IsNaN(surprise) || math.IsInf(surprise, 0) {
		return nil, errnie.Err(errnie.Internal, fmt.Sprintf(
			"[gate] %s surprise %g at %s is not finite", symbol.Symbol, surprise, tick.Prefix,
		), nil)
	}

	if moments.settle.fastRate == 0 {
		moments.settle.fastRate, moments.settle.slowRate = gate.settings.Warmup.FastRate, gate.settings.Warmup.SlowRate
	}

	out := &GateReading{Surprise: surprise}

	if out.Warm, out.Cold = moments.settle.observe(surprise); !out.Warm {
		return out, nil
	}

	if moments.count >= 2 {
		deviation := math.Sqrt(moments.m2 / float64(moments.count-1))

		if !(deviation > 0) {
			return nil, errnie.Err(errnie.Internal, fmt.Sprintf(
				"[gate] %s surprise has not varied over %d warm solver steps (at %s): no z-score to gate by",
				symbol.Symbol, moments.count, tick.Prefix,
			), nil)
		}

		z := (surprise - moments.mean) / deviation
		out.Z = &z
	}

	out.Passed = out.Z != nil && *out.Z >= gate.settings.SurpriseZ

	moments.count++
	delta := surprise - moments.mean
	moments.mean += delta / float64(moments.count)
	moments.m2 += delta * (surprise - moments.mean)

	return out, nil
}
