package logic

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
stepManifold appends tokenized book samples for changed Hawkes epochs into the
resident Sensorium history and always advances the shared GPU field once for
this tick.
*/
func (analyzer *Analyzer) stepManifold(
	thesis *types.Thesis,
	hawkes manifold.HawkesSource,
	cutID types.CutID,
	tick int64,
) {
	switch {
	case analyzer.manifold == nil:
		payload := map[string]any{"ok": false, "skip": "manifold_nil"}

		if cutID > 0 {
			payload["cut_id"] = uint64(cutID)
		}

		errnie.Error(audit.Phase(analyzer.recorder, tick, "manifold", payload))
		return
	case hawkes == nil:
		payload := map[string]any{"ok": false, "skip": "hawkes_nil"}

		if cutID > 0 {
			payload["cut_id"] = uint64(cutID)
		}

		errnie.Error(audit.Phase(analyzer.recorder, tick, "manifold", payload))
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

	if cutID > 0 {
		payload["cut_id"] = uint64(cutID)
	}

	errnie.Error(audit.Phase(analyzer.recorder, tick, "manifold", payload))
}
