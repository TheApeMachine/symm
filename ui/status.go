package ui

import "time"

/*
Status publishes wallet and open-position telemetry for the dashboard header.
*/
func (stream *MarketStream) Status(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "status"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}

/*
DecisionTrace publishes one scored decision cycle for the sidebar.
*/
func (stream *MarketStream) DecisionTrace(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "decision_trace"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}
