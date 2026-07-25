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
Resonance and causal go first so a saturated UI channel drops manifold after
the lighter charts when the ingress buffer is full. Thesis rows stay book-wide
for strategy; UI frames are focus-gated so the socket does not ship every
symbol each cut. Manifold publishes one shared-field row; focus only labels
that row for client inheritance.
*/
func (analyzer *Analyzer) publishMeasured(
	thesis *types.Thesis,
	states []manifold.State,
) {
	publishStarted := time.Now()

	if framed := focusRows(thesis.Resonance); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"resonance": framed})
	}

	if framed := focusRows(thesis.Causal); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"causal": framed})
	}

	if framed := focusHypotheses(thesis.Hypotheses); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"hypotheses": framed})
	}

	if wired := wireManifold(states); len(wired) > 0 {
		field, particles, wave := manifold.WirePackets(wired[0])
		analyzer.publish(datura.Map[any]{"manifold": []manifold.WireField{field}})
		analyzer.publish(datura.Map[any]{"manifold_particles": particles})
		analyzer.publish(datura.Map[any]{"manifold_wave": wave})
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "publish", map[string]any{
		"ns":       time.Since(publishStarted).Nanoseconds(),
		"manifold": len(states),
	}))
}

/*
wireManifold publishes one row carrying the shared Sensorium field. Focus only
chooses which symbol label the row wears for client-side inheritance; ρ, |ψ|²,
guidance, wave, and the resident particle cloud are shared physics and always
travel with that row — the manifold is not a per-symbol mechanism.
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
	selected.Particles = states[field].Particles
	selected.OscillatorCount = states[field].OscillatorCount
	selected.SharedOscillatorCount = states[field].SharedOscillatorCount

	return []manifold.State{selected}
}

/*
publishCognition emits cognition and forecast frames after REM consolidation,
and calibrates already-composed thesis.Categories with DMT surprisal/confidence.
Thesis.Categories stays book-wide for strategy; the categories UI frame is
focus-gated like cognition so the rail does not ship every symbol each cut.
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

	analyzer.calibrateCategories(thesis, cognition)

	if framed := focusCognition(cognition); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": framed})
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}

	if framed := focusCategories(thesis.Categories); len(framed) > 0 {
		analyzer.publish(datura.Map[any]{"categories": framed})
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
focusCategories returns every composed category when focus is unset, otherwise
only the focused symbol so the thesis rail paints without a book-wide flood.
*/
func focusCategories(categories []types.Category) []types.Category {
	focus := types.Focus()

	if focus == "" {
		return categories
	}

	framed := make([]types.Category, 0, 8)

	for _, category := range categories {
		if category.Symbol != focus {
			continue
		}

		framed = append(framed, category)
	}

	return framed
}

/*
focusHypotheses returns every hypothesis when focus is unset, otherwise only the
focused symbol so the thesis rail does not ship the full book each cut.
*/
func focusHypotheses(hypotheses []types.Hypothesis) []types.Hypothesis {
	focus := types.Focus()

	if focus == "" {
		return hypotheses
	}

	framed := make([]types.Hypothesis, 0, 1)

	for _, hypothesis := range hypotheses {
		if hypothesis.Symbol != focus {
			continue
		}

		framed = append(framed, hypothesis)
	}

	return framed
}

/*
focusRows returns every resonance/causal row when focus is unset, otherwise only
the focused symbol so predictive-coding and causal charts stay lean on the wire.
*/
func focusRows(rows []any) []any {
	focus := types.Focus()

	if focus == "" {
		return rows
	}

	framed := make([]any, 0, 1)

	for _, row := range rows {
		if rowSymbol(row) != focus {
			continue
		}

		framed = append(framed, row)
	}

	return framed
}

/*
rowSymbol reads the market symbol from a resonance or causal thesis row.
*/
func rowSymbol(row any) string {
	switch value := row.(type) {
	case *ResonanceOutcome:
		if value == nil {
			return ""
		}

		return value.Symbol
	case ResonanceOutcome:
		return value.Symbol
	case *CausalOutcome:
		if value == nil {
			return ""
		}

		return value.Symbol
	case CausalOutcome:
		return value.Symbol
	}

	return ""
}

/*
calibrateCategories stamps DMT surprisal and confidence onto the strongest
composed category for each ready cognition symbol. Categories themselves come
from measurement×affinity composition, not buy/sell attractor labels.
*/
func (analyzer *Analyzer) calibrateCategories(
	thesis *types.Thesis,
	cognition []types.Cognition,
) {
	if thesis == nil {
		return
	}

	for _, reading := range cognition {
		if !reading.Ready || reading.Symbol == "" {
			continue
		}

		best := -1

		for index := range thesis.Categories {
			if thesis.Categories[index].Symbol != reading.Symbol {
				continue
			}

			if best < 0 || thesis.Categories[index].Strength > thesis.Categories[best].Strength {
				best = index
			}
		}

		if best < 0 {
			continue
		}

		thesis.Categories[best].Surprisal = reading.EntropyBits

		if reading.Confidence > thesis.Categories[best].Confidence {
			thesis.Categories[best].Confidence = reading.Confidence
		}

		if reading.LookaheadScore > thesis.Categories[best].Strength {
			thesis.Categories[best].Maturity = float64(reading.Cohort)
		}
	}
}
