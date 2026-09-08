package model

import "github.com/theapemachine/errnie"

/*
Feedback teaches a provisional objective increment without resolving the action.
Completed outcomes take precedence over provisional evidence for that action.
Repeated feedback is correlated trajectory evidence, not independent trials.
*/
func (model *Model[Key, Action]) Feedback(identity uint64, value float64) error {
	pending, exists := model.pending[identity]

	if !exists {
		return errnie.Error(errnie.Err(
			errnie.Validation, "model: feedback requires an unresolved action", nil,
		))
	}

	for _, record := range pending.priors {
		if err := record.Provisional.Observe(value, pending.authority, record.Memory, *record.epoch); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}
