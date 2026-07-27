package logic

import (
	"sync"
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
the lighter charts when the ingress buffer is full. Thesis rows and UI frames
stay book-wide so allocation, decisions, and cross-section panels can see the
same universe the strategy saw. Manifold publishes one shared-field row plus a
binary display texture and wave packet so the browser blits the GPU result.
*/
func (analyzer *Analyzer) publishMeasured(
	thesis *types.Thesis,
	states []manifold.State,
	cutID types.CutID,
	tick int64,
) {
	if analyzer.ui == nil {
		return
	}

	publishStarted := time.Now()

	analyzer.frameRows = focusRowsInto(analyzer.frameRows[:0], thesis.Resonance)

	if len(analyzer.frameRows) > 0 {
		analyzer.publish(datura.NewMap("resonance", analyzer.frameRows))
	}

	analyzer.frameRows = focusRowsInto(analyzer.frameRows[:0], thesis.Causal)

	if len(analyzer.frameRows) > 0 {
		analyzer.publish(datura.NewMap("causal", analyzer.frameRows))
	}

	analyzer.hypRows = focusHypothesesInto(analyzer.hypRows[:0], thesis.Hypotheses)

	if len(analyzer.hypRows) > 0 {
		analyzer.publish(datura.NewMap("hypotheses", analyzer.hypRows))
	}

	if wired, ok := wireManifoldState(states); ok {
		field, displays, wave := wired.WirePackets()
		analyzer.publish(datura.NewMap("manifold", []manifold.WireField{field}))

		for _, display := range displays {
			if len(display) > 0 {
				analyzer.publishBytes(display)
			}
		}

		analyzer.publish(datura.NewMap("manifold_wave", wave))
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
Thesis.Categories stays book-wide for strategy and UI so cortex, xray, and
allocation all inspect the same ranked universe.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	if analyzer.ui == nil || thesis == nil {
		return
	}

	analyzer.cogRows = analyzer.cogRows[:0]
	focus := types.Focus()

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if !ok {
			return true
		}

		if focus == "" || reading.Symbol == focus {
			analyzer.cogRows = append(analyzer.cogRows, reading)

			if focus != "" {
				return false
			}
		}

		return true
	})

	if len(analyzer.cogRows) > 0 {
		analyzer.publish(datura.NewMap("cognition", analyzer.cogRows))
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.NewMap("forecasts", thesis.Forecasts))
	}

	analyzer.catRows = analyzer.catRows[:0]

	for _, rows := range thesis.Categories {
		analyzer.catRows = append(analyzer.catRows, rows...)
	}

	if len(analyzer.catRows) > 0 {
		analyzer.publish(datura.NewMap("categories", analyzer.catRows))
	}
}

func focusHypothesesInto(dst []types.Hypothesis, hypotheses []types.Hypothesis) []types.Hypothesis {
	return append(dst[:0], hypotheses...)
}

func focusRowsInto(dst []any, rows *sync.Map) []any {
	dst = dst[:0]
	rows.Range(func(_, row any) bool {
		dst = append(dst, row)

		return true
	})

	return dst
}

