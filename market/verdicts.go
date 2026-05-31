package market

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
EntryVerdict is one flat-entry playbook outcome including its decision trace.
ReleaseEntryVerdicts when done to return traces to sync.Pool.
*/
type EntryVerdict struct {
	Name   string
	Action perspectives.ActionType
	Regime perspectives.Regime
	Trace  *perspectives.DecisionTrace
}

/*
EntryVerdicts evaluates every registered playbook and returns all actionable
flat-entry outcomes, including deny and wait gates omitted by Decisions().
*/
func EntryVerdicts(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
) []EntryVerdict {
	return EntryVerdictsWithContext(measurements, observations, nil)
}

func EntryVerdictsWithContext(
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
	context func(string) perspectives.DecisionContext,
) []EntryVerdict {
	verdicts := make([]EntryVerdict, 0, len(perspectiveRegistry))

	for _, entry := range perspectiveRegistry {
		trace := perspectives.AcquireTrace(entry.perspective.Name())
		decisionContext := perspectives.DecisionContext{}

		if context != nil {
			decisionContext = context(entry.name)
		}

		action := decideWithTrace(
			entry.perspective, measurements, observations, trace, decisionContext,
		)

		if action == nil {
			perspectives.ReleaseTrace(trace)

			continue
		}

		trace.FinalAction = *action
		verdicts = append(verdicts, EntryVerdict{
			Name:   entry.name,
			Action: *action,
			Regime: entry.perspective.Regime(),
			Trace:  trace,
		})
	}

	return verdicts
}

/*
ReleaseEntryVerdicts returns pooled traces from EntryVerdicts results.
*/
func ReleaseEntryVerdicts(verdicts []EntryVerdict) {
	for _, verdict := range verdicts {
		perspectives.ReleaseTrace(verdict.Trace)
	}
}

type traceDecider interface {
	DecideWithTrace(
		measurements []perspectives.Measurement,
		observations []perspectives.ObservationType,
		trace *perspectives.DecisionTrace,
	) *perspectives.ActionType
}

func decideWithTrace(
	perspective perspectives.Perspective,
	measurements []perspectives.Measurement,
	observations []perspectives.ObservationType,
	trace *perspectives.DecisionTrace,
	context perspectives.DecisionContext,
) *perspectives.ActionType {
	if decider, ok := perspective.(contextTraceDecider); ok {
		return decider.DecideWithTraceContext(measurements, observations, trace, context)
	}

	if decider, ok := perspective.(traceDecider); ok {
		return decider.DecideWithTrace(measurements, observations, trace)
	}

	return perspective.Decide(measurements, observations)
}

type contextTraceDecider interface {
	DecideWithTraceContext(
		measurements []perspectives.Measurement,
		observations []perspectives.ObservationType,
		trace *perspectives.DecisionTrace,
		context perspectives.DecisionContext,
	) *perspectives.ActionType
}
