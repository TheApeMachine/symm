package audit

var strategyLogicPhases = map[string]struct{}{
	"categories_commit": {},
	"cognize":           {},
	"desk":              {},
	"decide_end":        {},
	"manifold":          {},
	"measure_end":       {},
	"observe":           {},
	"rem":               {},
	"rem_begin":         {},
	"rem_deferred":      {},
	"tick_end":          {},
}

/*
Phase writes one ordered runtime breadcrumb. Tick correlates the row with the
crypto sequence counter so a freeze can be located by the last phase that
landed before the next tick row.
*/
func Phase(
	recorder *Recorder,
	tick int64,
	phase string,
	value map[string]any,
) error {
	if recorder == nil {
		return nil
	}

	if _, keep := strategyLogicPhases[phase]; !keep {
		return nil
	}

	if value == nil {
		value = map[string]any{}
	}

	value["tick"] = tick
	value["phase"] = phase

	return Record(recorder, "phase", value)
}
