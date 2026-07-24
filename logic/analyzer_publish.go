package logic

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
publishMeasured emits manifold, resonance, causal, and hypothesis frames.
Resonance and causal go first so a saturated UI channel cannot drop them behind
a manifold fan-out. Manifold publishes one focus-aware batch so the pilot-wave
painter can inherit ρ/|ψ|² from the field carrier onto the focused symbol.
*/
func (analyzer *Analyzer) publishMeasured(
	thesis *types.Thesis,
	states []manifold.State,
) {
	publishStarted := time.Now()

	if len(thesis.Resonance) > 0 {
		analyzer.publish(datura.Map[any]{"resonance": thesis.Resonance})
	}

	if len(thesis.Causal) > 0 {
		analyzer.publish(datura.Map[any]{"causal": thesis.Causal})
	}

	if len(thesis.Hypotheses) > 0 {
		analyzer.publish(datura.Map[any]{"hypotheses": thesis.Hypotheses})
	}

	if wired := wireManifold(states); len(wired) > 0 {
		analyzer.publish(datura.Map[any]{"manifold": wired})
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "publish", map[string]any{
		"ns":       time.Since(publishStarted).Nanoseconds(),
		"manifold": len(states),
	}))
}

/*
wireManifold keeps shared Sensorium lattices on one published row. The dashboard
focus symbol is preferred as that row when present; ρ/|ψ|² are copied from the
first state that still carries the field so focus without local lattices still
paints the pilot-wave. Non-focus particle clouds are stripped.
*/
func wireManifold(states []manifold.State) []manifold.State {
	if len(states) == 0 {
		return nil
	}

	field := -1

	for index, state := range states {
		if len(state.Rho) > 0 {
			field = index
			break
		}
	}

	if field < 0 {
		field = 0
	}

	target := field
	focus := types.Focus()

	if focus != "" {
		for index, state := range states {
			if state.Symbol == focus {
				target = index
				break
			}
		}
	}

	selected := states[target]
	selected.Rho = states[field].Rho
	selected.PsiMag2 = states[field].PsiMag2
	selected.GuidanceVelX = states[field].GuidanceVelX
	selected.GuidanceVelZ = states[field].GuidanceVelZ
	selected.Wave = states[field].Wave

	if target == field {
		selected.Particles = states[field].Particles
	} else {
		selected.Particles = nil
	}

	return []manifold.State{selected}
}

/*
publishCognition emits cognition and forecast frames after REM consolidation,
and projects ready winners onto thesis.Categories so the terminal category rail
is not an empty shell while classifications live only on Cognition. Strategy
sees every ready winner; the UI frame is focus-gated when a focus is set.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	cognition := make([]types.Cognition, 0, 8)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if !ok {
			return true
		}

		cognition = append(cognition, reading)

		return true
	})

	thesis.Categories = nil
	analyzer.projectCategories(thesis, cognition)

	if framed := focusCognition(cognition); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": framed})
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}

	if len(thesis.Categories) > 0 {
		analyzer.publish(datura.Map[any]{"categories": thesis.Categories})
	}
}

/*
focusCognition returns every reading when focus is unset, otherwise only the
focused symbol so cognitive surfaces paint one coherent tree instead of a flood.
*/
func focusCognition(cognition []types.Cognition) []types.Cognition {
	focus := types.Focus()

	if focus == "" {
		return cognition
	}

	framed := make([]types.Cognition, 0, 1)

	for _, reading := range cognition {
		if reading.Symbol != focus {
			continue
		}

		framed = append(framed, reading)
	}

	return framed
}

/*
projectCategories writes one Category row per ready cognition winner so strategy
publish and the thesis modal share the same classification surface.
*/
func (analyzer *Analyzer) projectCategories(
	thesis *types.Thesis,
	cognition []types.Cognition,
) {
	if thesis == nil {
		return
	}

	for _, reading := range cognition {
		if !reading.Ready || reading.Winner == "" || reading.Symbol == "" {
			continue
		}

		category := types.Category{
			Symbol:     reading.Symbol,
			Type:       types.CategoryType(reading.Winner),
			Confidence: reading.Confidence,
			Surprisal:  reading.EntropyBits,
			Strength:   reading.LookaheadScore,
			Maturity:   float64(reading.Cohort),
		}

		analyzer.attachCategoryEvidence(thesis, &category)
		thesis.Categories = append(thesis.Categories, category)
	}
}

/*
attachCategoryEvidence is a no-op while the resident evidence graph is removed.
Cognition winners still publish; supporting/opposing lists stay empty until the
market graph is redesigned.
*/
func (analyzer *Analyzer) attachCategoryEvidence(
	_ *types.Thesis,
	_ *types.Category,
) {
}
