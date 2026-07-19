package logic

import (
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
publishMeasured emits manifold, resonance, causal, and hypothesis frames.
Focused symbols keep full field payloads; the rest ship Summary scalars only.
*/
func (analyzer *Analyzer) publishMeasured(
	thesis *types.Thesis,
	states []manifold.State,
) {
	publishStarted := time.Now()

	if len(states) > 0 {
		focus := viper.GetString("ui.manifold_focus")
		frame := make([]any, 0, len(states))

		for _, state := range states {
			if focus != "" && state.Symbol == focus {
				frame = append(frame, state)
				continue
			}

			frame = append(frame, state.Summary())
		}

		analyzer.publish(datura.Map[any]{"manifold": frame})
	}

	if len(thesis.Resonance) > 0 {
		analyzer.publish(datura.Map[any]{"resonance": thesis.Resonance})
	}

	if len(thesis.Causal) > 0 {
		analyzer.publish(datura.Map[any]{"causal": thesis.Causal})
	}

	if len(thesis.Hypotheses) > 0 {
		analyzer.publish(datura.Map[any]{"hypotheses": thesis.Hypotheses})
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "publish", map[string]any{
		"ns": time.Since(publishStarted).Nanoseconds(),
	}))
}

/*
publishCognition emits cognition and forecast frames after REM consolidation,
and projects ready winners onto thesis.Categories so the terminal category rail
is not an empty shell while classifications live only on Cognition.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	cognition := make([]types.Cognition, 0, 8)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if ok {
			cognition = append(cognition, reading)
		}

		return true
	})

	if len(cognition) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": cognition})
		analyzer.projectCategories(thesis, cognition)
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}

	if len(thesis.Categories) > 0 {
		analyzer.publish(datura.Map[any]{"categories": thesis.Categories})
	}
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

		thesis.Categories = append(thesis.Categories, types.Category{
			Symbol:     reading.Symbol,
			Type:       types.CategoryType(reading.Winner),
			Confidence: reading.Confidence,
			Surprisal:  reading.EntropyBits,
			Strength:   reading.LookaheadScore,
			Maturity:   float64(reading.Cohort),
		})
	}
}

/*
publishGraphs emits composed evidence-graph wire frames so the thesis modal can
render measurement relationships for the focused symbol.
*/
func (analyzer *Analyzer) publishGraphs(thesis *types.Thesis) {
	if thesis == nil || thesis.Graphs == nil {
		return
	}

	frames := make([]types.GraphFrame, 0)

	thesis.Graphs.Range(func(_, value any) bool {
		evidenceGraph, ok := value.(*types.Graph)

		if !ok || evidenceGraph == nil {
			return true
		}

		frame := evidenceGraph.Frame()

		if len(frame.Nodes) == 0 {
			return true
		}

		frames = append(frames, frame)

		return true
	})

	if len(frames) == 0 {
		return
	}

	analyzer.publish(datura.Map[any]{"graphs": frames})
}
