package logic

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
stepManifold advances the Hawkes-driven GPU field when preflight is ready.
*/
func (analyzer *Analyzer) stepManifold(thesis *types.Thesis) {
	if analyzer.manifold == nil ||
		analyzer.hawkes == nil ||
		analyzer.gate == nil ||
		!analyzer.gate.Ready(system.StagePreflight) {
		return
	}

	manifoldStarted := time.Now()
	payload := map[string]any{"ok": true}

	if err := analyzer.manifold.Update(
		thesis,
		analyzer.hawkes,
		analyzer.Focused(),
	); err != nil {
		errnie.Error(err)
		payload["ok"] = false
		payload["err"] = err.Error()
	}

	payload["ns"] = time.Since(manifoldStarted).Nanoseconds()
	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "manifold", payload))
}
