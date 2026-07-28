package strategy

import "github.com/theapemachine/symm/types"

/*
syncThesisLifecycle projects live desk holdings back onto the Thesis lifecycle
surface so the strategy and UI describe the same real lot state instead of
freezing at submit-time phases.
*/
func syncThesisLifecycle(
	thesis *types.Thesis,
	live map[string]*types.Holding,
) {
	if thesis == nil {
		return
	}

	for symbol, holding := range live {
		current, _ := lifecycleState(thesis, symbol)
		next := holdingLifecycle(current, holding)

		if next != "" && next != current {
			thesis.NoteLifecycle(symbol, next, thesis.At)
		}
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		if _, found := live[symbol]; found {
			return true
		}

		current, _ := value.(string)

		switch current {
		case types.LifecycleEntered,
			types.LifecycleManaging,
			types.LifecycleExitSelected,
			types.LifecycleExitSubmitted,
			types.LifecyclePartiallyExited:
			thesis.NoteLifecycle(symbol, types.LifecycleClosed, thesis.At)
		}

		return true
	})
}

func lifecycleState(thesis *types.Thesis, symbol string) (string, bool) {
	if thesis == nil || symbol == "" {
		return "", false
	}

	value, found := thesis.Lifecycle.Load(symbol)

	if !found {
		return "", false
	}

	state, ok := value.(string)

	return state, ok
}

func holdingLifecycle(current string, holding *types.Holding) string {
	if holding == nil {
		return current
	}

	switch holding.Status {
	case types.OPEN, types.FILLED, types.READY:
		if exitingLifecycle(current) {
			return current
		}

		return types.LifecycleManaging
	case types.PARTIAL, types.PARTIAL_FILLED,
		types.PARTIAL_CANCELLED, types.PARTIAL_REJECTED,
		types.PARTIAL_EXPIRED:
		if exitingLifecycle(current) {
			return types.LifecyclePartiallyExited
		}

		return types.LifecyclePartiallyEntered
	case types.PENDING, types.NEW, types.PRIORITY:
		if exitingLifecycle(current) {
			return types.LifecycleExitSubmitted
		}

		return types.LifecycleEntrySubmitted
	case types.CLOSED:
		return types.LifecycleClosed
	case types.CANCELED:
		if exitingLifecycle(current) {
			return types.LifecycleManaging
		}

		return types.LifecycleInvalid
	case types.REJECTED:
		return types.LifecycleRejected
	case types.EXPIRED:
		return types.LifecycleExpired
	case types.ERROR, types.FATAL:
		return types.LifecycleInvalid
	default:
		return current
	}
}

func exitingLifecycle(current string) bool {
	switch current {
	case types.LifecycleExitSelected,
		types.LifecycleExitSubmitted,
		types.LifecyclePartiallyExited:
		return true
	}

	return false
}
