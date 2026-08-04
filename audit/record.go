package audit

var diagnosticEventTypes = map[string]struct{}{
	"decision":   {},
	"phase":      {},
	"predictive": {},
	"position":   {},
	/*
		stop rows carry one change in a regulator's geometry. They exist to be
		replayed later: every row states where the hard floor and the profit line
		stood and where the mark was against them, which is what labels whether a
		lot reached its profit before its loss. That question cannot be answered
		from the decision rows, because the stop acts between them.
	*/
	"stop": {},
	/*
		passage rows are one complete finished lot: its features at every state
		it passed through, the boundary it reached first, and its extremes. They
		are what an offline fit needs to replace the configured risk multiples
		with a measured adverse-excursion quantile, and the in-memory model
		cannot be that corpus — it is bounded and dies with the process.
	*/
	"passage": {},
}

/*
Record writes one structured diagnostic row to the audit jsonl file.
Diagnostic rows use channel "diagnostic" so they can be filtered apart from
bus traffic mirrors (measurements, raw, ui).
*/
func Record(recorder *Recorder, eventType string, value any) error {
	if recorder == nil {
		return nil
	}

	if _, keep := diagnosticEventTypes[eventType]; !keep {
		return nil
	}

	return recorder.Write(map[string]any{
		"channel": "diagnostic",
		"type":    eventType,
		"value":   value,
	})
}
