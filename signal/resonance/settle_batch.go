package resonance

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func (signal *Signal[T]) SettleScopes(scopes []string) (map[string]*types.Measurement, error) {
	// Size the engine to the full live universe this tick so every symbol gets
	// a slot — the count is discovered, never a fixed cap.
	if err := signal.ensureCapacity(len(scopes)); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: failed to ensure engine",
			err,
		))
	}

	// Release the slots of symbols that left the universe so a rotating set of
	// pairs reuses slots instead of leaking them, and clear their learned state
	// so the next symbol settling into a reused slot starts fresh.
	if freed := signal.slots.retain(scopes); len(freed) > 0 {
		if resetErr := signal.engine.Reset(freed); resetErr != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"resonance: failed to reset reclaimed slots",
				resetErr,
			))
		}
	}

	changedScopes := signal.filterChangedScopes(scopes)

	if len(changedScopes) == 0 {
		return map[string]*types.Measurement{}, nil
	}

	entries, contexts := signal.collectBatchEntries(changedScopes)

	if len(entries) == 0 {
		return map[string]*types.Measurement{}, nil
	}

	results := errnie.Does(func() (map[string]*types.Measurement, error) {
		return signal.runBatchSettle(entries, contexts)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: failed to run batch settle",
			err,
		))
	})

	if results.Err() != nil {
		return nil, results.Err()
	}

	signal.rememberSettledScopes(changedScopes)

	return results.Value(), nil
}

func (signal *Signal[T]) collectBatchEntries(
	scopes []string,
) ([]batchEntry, map[string]featureContext) {
	entries := make([]batchEntry, 0, len(scopes))
	contexts := make(map[string]featureContext, len(scopes))

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		features, ok := signal.featureContext(scope)

		if !ok {
			continue
		}

		slot, ok := signal.slots.assign(scope)

		if !ok {
			continue
		}

		entries = append(entries, batchEntry{
			slot:   slot,
			symbol: scope,
			input:  features.input,
		})

		contexts[scope] = features
	}

	return entries, contexts
}

func (signal *Signal[T]) runBatchSettle(
	entries []batchEntry,
	contexts map[string]featureContext,
) (map[string]*types.Measurement, error) {
	outcomes, err := signal.engine.Settle(entries)

	if err != nil {
		signal.err = err

		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"resonance: failed to settle batch",
			err,
		))
	}

	results, settled := signal.buildSettleResults(outcomes, contexts)

	if err := signal.publishUniverse(settled); err != nil {
		signal.err = errnie.Error(err)
	}

	signal.lastSettled = settled

	return results, signal.err
}

func (signal *Signal[T]) buildSettleResults(
	outcomes []settleOutcome,
	contexts map[string]featureContext,
) (map[string]*types.Measurement, []settledSymbolEntry) {
	results := make(map[string]*types.Measurement, len(outcomes))
	settled := make([]settledSymbolEntry, 0, len(outcomes))

	for _, outcome := range outcomes {
		features, ok := contexts[outcome.symbol]

		if !ok {
			continue
		}

		measurement, err := signal.measurementFromOutcome(outcome, features)

		if err != nil {
			signal.err = errnie.Error(err)
			continue
		}

		results[outcome.symbol] = measurement

		wire, err := buildSettledSymbolEntry(signal, outcome, measurement)

		if err != nil {
			signal.err = errnie.Error(err)
			continue
		}

		settled = append(settled, wire)
	}

	return results, settled
}
