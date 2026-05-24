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

/*
TradeEnter publishes one paper position open event.
*/
func (stream *MarketStream) TradeEnter(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "trade_enter"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}

/*
TradeExit publishes one paper position close event.
*/
func (stream *MarketStream) TradeExit(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "trade_exit"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}

/*
StopRatchet publishes one trailing-stop update for an open position.
*/
func (stream *MarketStream) StopRatchet(payload map[string]any) {
	if stream == nil {
		return
	}

	payload["event"] = "stop_ratchet"
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	stream.Emit(payload)
}
