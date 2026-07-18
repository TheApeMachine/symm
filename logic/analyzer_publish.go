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
publishCognition emits cognition and forecast frames after REM consolidation.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	cognition := make([]types.Cognition, 0)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if ok {
			cognition = append(cognition, reading)
		}

		return true
	})

	if len(cognition) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": cognition})
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}
}
