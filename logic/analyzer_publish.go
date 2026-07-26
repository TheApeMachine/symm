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
	cutID types.CutID,
	tick int64,
) {
	publishStarted := time.Now()

	analyzer.frameRows = focusRowsInto(analyzer.frameRows[:0], thesis.Resonance)

	if len(analyzer.frameRows) > 0 {
		analyzer.publish(datura.Map[any]{"resonance": analyzer.frameRows})
	}

	analyzer.frameRows = focusRowsInto(analyzer.frameRows[:0], thesis.Causal)

	if len(analyzer.frameRows) > 0 {
		analyzer.publish(datura.Map[any]{"causal": analyzer.frameRows})
	}

	analyzer.hypRows = focusHypothesesInto(analyzer.hypRows[:0], thesis.Hypotheses)

	if len(analyzer.hypRows) > 0 {
		analyzer.publish(datura.Map[any]{"hypotheses": analyzer.hypRows})
	}

	if wired, ok := wireManifoldState(states); ok {
		field, _, wave := wired.WirePackets()
		analyzer.publish(datura.Map[any]{"manifold": []manifold.WireField{field}})

		analyzer.publish(datura.Map[any]{"manifold_wave": wave})
	}

	payload := map[string]any{
		"ns":       time.Since(publishStarted).Nanoseconds(),
		"manifold": len(states),
	}

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "publish", payload))
}

func wireManifoldState(states []manifold.State) (manifold.State, bool) {
	if len(states) == 0 {
		return manifold.State{}, false
	}

	field := -1

	for index, state := range states {
		if len(state.Display) > 0 {
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
	selected.Display = states[field].Display
	selected.DisplayWidth = states[field].DisplayWidth
	selected.DisplayHeight = states[field].DisplayHeight
	selected.RhoOccupied = states[field].RhoOccupied
	selected.PsiOccupied = states[field].PsiOccupied
	selected.RhoMax = states[field].RhoMax
	selected.PsiMax = states[field].PsiMax
	selected.Grid = states[field].Grid
	selected.Wave = states[field].Wave
	selected.OscillatorCount = states[field].OscillatorCount
	selected.SharedOscillatorCount = states[field].SharedOscillatorCount

	return selected, true
}

/*
publishCognition emits cognition and forecast frames after REM consolidation,
and calibrates already-composed thesis.Categories with DMT surprisal/confidence.
Thesis.Categories stays book-wide for strategy; the categories UI frame is
focus-gated like cognition so the rail does not ship every symbol each cut.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	focus := types.Focus()
	analyzer.cogRows = analyzer.cogRows[:0]

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if !ok {
			return true
		}

		if focus == "" || reading.Symbol == focus {
			analyzer.cogRows = append(analyzer.cogRows, reading)
		}

		return true
	})

	if len(analyzer.cogRows) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": analyzer.cogRows})
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}

	analyzer.catRows = thesis.Categories[types.Focus()]

	if len(analyzer.catRows) > 0 {
		analyzer.publish(datura.Map[any]{"categories": analyzer.catRows})
	}
}

func focusHypothesesInto(dst []types.Hypothesis, hypotheses []types.Hypothesis) []types.Hypothesis {
	focus := types.Focus()

	if focus == "" {
		return hypotheses
	}

	dst = dst[:0]

	for _, hypothesis := range hypotheses {
		if hypothesis.Symbol != focus {
			continue
		}

		dst = append(dst, hypothesis)
	}

	return dst
}

func focusRowsInto(dst []any, rows []any) []any {
	focus := types.Focus()

	if focus == "" {
		return rows
	}

	dst = dst[:0]

	for _, row := range rows {
		if rowSymbol(row) != focus {
			continue
		}

		dst = append(dst, row)
	}

	return dst
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
