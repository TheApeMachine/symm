package logic

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
stepManifold appends tokenized book samples for changed Hawkes epochs into the
resident Sensorium history and advances the shared GPU field when preflight is
ready.
*/
func (analyzer *Analyzer) stepManifold(
	thesis *types.Thesis,
	hawkes manifold.HawkesSource,
) {
	if analyzer.manifold == nil ||
		hawkes == nil ||
		analyzer.gate == nil ||
		!analyzer.gate.Ready(system.StagePreflight) {
		return
	}

	manifoldStarted := time.Now()
	payload := map[string]any{"ok": true}

	if err := analyzer.manifold.Update(thesis, hawkes); err != nil {
		errnie.Error(err)
		payload["ok"] = false
		payload["err"] = err.Error()
	}

	payload["ns"] = time.Since(manifoldStarted).Nanoseconds()
	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "manifold", payload))
}
