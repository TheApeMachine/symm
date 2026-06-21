package resonance

import (
	"fmt"

	"github.com/theapemachine/datura"
)

func (signal *Signal) SettleScopes(scopes []string) (map[string]*datura.Artifact, error) {
	if signal == nil {
		return nil, fmt.Errorf("resonance: signal is nil")
	}

	signal.hydrateMarketFromTree()

	if ensureErr := signal.ensureEngine(); ensureErr != nil {
		return nil, ensureErr
	}

	entries, contexts := signal.collectBatchEntries(scopes)

	if len(entries) == 0 {
		return map[string]*datura.Artifact{}, signal.err
	}

	return signal.runBatchSettle(entries, contexts)
}

func (signal *Signal) collectBatchEntries(
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

func (signal *Signal) runBatchSettle(
	entries []batchEntry,
	contexts map[string]featureContext,
) (map[string]*datura.Artifact, error) {
	outcomes, settleErr := signal.engine.Settle(entries)

	if settleErr != nil {
		signal.err = settleErr

		return nil, settleErr
	}

	results, settled := signal.buildSettleResults(outcomes, contexts)

	if publishErr := signal.publishUniverse(settled); publishErr != nil {
		signal.err = publishErr
	}

	signal.lastSettled = settled

	return results, signal.err
}

func (signal *Signal) buildSettleResults(
	outcomes []settleOutcome,
	contexts map[string]featureContext,
) (map[string]*datura.Artifact, []settledSymbolEntry) {
	results := make(map[string]*datura.Artifact, len(outcomes))
	settled := make([]settledSymbolEntry, 0, len(outcomes))

	for _, outcome := range outcomes {
		features, ok := contexts[outcome.symbol]

		if !ok {
			continue
		}

		measurement, publishable := signal.measurementFromOutcome(outcome, features)

		if !publishable || measurement == nil {
			continue
		}

		results[outcome.symbol] = measurement

		wire, wireErr := buildSettledSymbolEntry(signal, outcome, measurement)

		if wireErr != nil {
			signal.err = wireErr

			continue
		}

		settled = append(settled, wire)
	}

	return results, settled
}
