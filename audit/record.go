package audit

import (
	"fmt"

	"github.com/bytedance/sonic"
)

// DecisionBatchEvent is the stable audit protocol name for retained decision moments.
const DecisionBatchEvent = "decision_batch"

// OutcomeBatchEvent is the stable audit protocol name for matured outcome tracking.
const OutcomeBatchEvent = "outcome_batch"

/*
Record validates and writes one typed analytical event: the curated audit
stream of decision moments. A recorder with an event sink writes there —
the sqlite audit tables — and never touches the file.
*/
func Record(recorder *Recorder, event any) error {
	return RecordAs(recorder, fmt.Sprintf("%T", event), event)
}

// RecordAs writes an analytical event under a stable protocol kind.
func RecordAs(recorder *Recorder, kind string, event any) error {
	if recorder == nil {
		return nil
	}

	if kind == "" {
		return fmt.Errorf("audit: event kind required")
	}

	if event == nil {
		return fmt.Errorf("audit: event required")
	}

	if recorder.EventSink != nil {
		payload, err := sonic.Marshal(event)

		if err != nil {
			return fmt.Errorf("audit: encode event: %w", err)
		}

		return recorder.EventSink(kind, payload)
	}

	return recorder.Write(map[string]any{
		"channel": "analysis",
		"type":    kind,
		"value":   event,
	})
}
