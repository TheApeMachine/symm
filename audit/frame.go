package audit

import "time"

/*
Frame is the typed contract for audit payloads written to JSONL.
*/
type Frame interface {
	Event() string
	Payload() map[string]any
}

/*
DedupedFrame adds a stable dedupe key enforced at compile time.
*/
type DedupedFrame interface {
	Frame
	DedupeKey() string
}

/*
DeadLetterFrame records discarded bus or desk events.
*/
type DeadLetterFrame struct {
	RecordedAt time.Time
	Kind       string
	Reason     string
	Detail     map[string]any
}

func (frame DeadLetterFrame) Event() string {
	return "dead_letter"
}

func (frame DeadLetterFrame) Payload() map[string]any {
	payload := map[string]any{
		"event":  frame.Event(),
		"ts":     frame.RecordedAt.Format(time.RFC3339Nano),
		"kind":   frame.Kind,
		"reason": frame.Reason,
	}

	for key, value := range frame.Detail {
		payload[key] = value
	}

	return payload
}

/*
DeskDecisionFrame is the typed audit payload for broker verdicts.
*/
type DeskDecisionFrame struct {
	RecordedAt time.Time
	Symbol     string
	ActionType string
	Side       string
	BranchKey  string
	Verdict    string
	Reason     string
}

func (frame DeskDecisionFrame) Event() string {
	return "desk_decision"
}

func (frame DeskDecisionFrame) DedupeKey() string {
	return "desk_reject:" + frame.Symbol + ":" + frame.ActionType + ":" + frame.Reason
}

func (frame DeskDecisionFrame) Payload() map[string]any {
	return map[string]any{
		"event":   frame.Event(),
		"ts":      frame.RecordedAt.Format(time.RFC3339Nano),
		"symbol":  frame.Symbol,
		"type":    frame.ActionType,
		"side":    frame.Side,
		"key":     frame.BranchKey,
		"verdict": frame.Verdict,
		"reason":  frame.Reason,
	}
}

func framePayload(frame Frame) map[string]any {
	if frame == nil {
		return nil
	}

	return frame.Payload()
}
