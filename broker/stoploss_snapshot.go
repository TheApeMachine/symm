package broker

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type stoplossSnapshot struct {
	State                 int       `json:"state"`
	StateLabel            string    `json:"state_label"`
	LastMark              float64   `json:"last_mark"`
	RecentMarks           []float64 `json:"recent_marks"`
	Peak                  float64   `json:"peak"`
	Stop                  float64   `json:"stop"`
	Offset                float64   `json:"offset"`
	Side                  string    `json:"side"`
	ArmedAt               string    `json:"armed_at"`
	Trigger               float64   `json:"trigger,omitempty"`
	TriggeredAt           string    `json:"triggered_at,omitempty"`
	ExitOrderID           string    `json:"exit_order_id,omitempty"`
	RetryCount            int       `json:"retry_count"`
	LastError             string    `json:"last_error,omitempty"`
	WaitingBalance        string    `json:"waiting_balance,omitempty"`
	NativeOrderID         string    `json:"native_order_id,omitempty"`
	NativeExchangeOrderID string    `json:"native_exchange_order_id,omitempty"`
	NativeState           string    `json:"native_state,omitempty"`
	NativeLastStatus      string    `json:"native_last_status,omitempty"`
}

func newStoplossSnapshot(state StoplossState, price float64, offset float64) *stoplossSnapshot {
	snapshot := &stoplossSnapshot{
		LastMark:    price,
		RecentMarks: []float64{price},
		Peak:        price,
		Stop:        price * (1 - offset),
		Offset:      offset,
		Side:        "sell",
		ArmedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	snapshot.setState(state)
	return snapshot
}

func (snapshot *stoplossSnapshot) setState(state StoplossState) {
	if snapshot == nil {
		return
	}

	snapshot.State = int(state)
	snapshot.StateLabel = stoplossStateLabel(state)
}

func stoplossState(order *datura.Artifact) *stoplossSnapshot {
	if order == nil {
		return nil
	}

	attributes, err := order.Attributes()
	if err != nil || len(attributes) == 0 {
		return nil
	}

	var root struct {
		Stoploss stoplossSnapshot `json:"stoploss"`
	}
	if err := sonic.Unmarshal(attributes, &root); err != nil {
		return nil
	}
	if root.Stoploss.State == 0 && root.Stoploss.StateLabel == "" {
		return nil
	}

	return &root.Stoploss
}

func writeStoplossState(order *datura.Artifact, state *stoplossSnapshot) {
	if order == nil || state == nil {
		return
	}

	root := make(map[string]any)
	attributes, err := order.Attributes()
	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "stoploss: read state attributes", err))
		return
	}
	if len(attributes) > 0 {
		if err := sonic.Unmarshal(attributes, &root); err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "stoploss: decode state attributes", err))
			return
		}
	}

	root["stoploss"] = state

	wire, err := sonic.Marshal(root)
	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "stoploss: marshal state", err))
		return
	}

	errnie.Error(order.SetAttributes(wire))
}

func appendStopMark(marks []float64, mark float64) []float64 {
	const capMarks = 64

	marks = append(marks, mark)
	if len(marks) <= capMarks {
		return marks
	}

	return marks[len(marks)-capMarks:]
}

func (snapshot *stoplossSnapshot) payload(symbol string) datura.Map[any] {
	return datura.Map[any]{
		"symbol":                   symbol,
		"state":                    snapshot.State,
		"state_label":              snapshot.StateLabel,
		"last_mark":                snapshot.LastMark,
		"recent_marks":             snapshot.RecentMarks,
		"peak":                     snapshot.Peak,
		"stop":                     snapshot.Stop,
		"offset":                   snapshot.Offset,
		"side":                     snapshot.Side,
		"trigger":                  snapshot.Trigger,
		"triggered_at":             snapshot.TriggeredAt,
		"exit_order_id":            snapshot.ExitOrderID,
		"retry_count":              snapshot.RetryCount,
		"last_error":               snapshot.LastError,
		"waiting_balance":          snapshot.WaitingBalance,
		"native_order_id":          snapshot.NativeOrderID,
		"native_exchange_order_id": snapshot.NativeExchangeOrderID,
		"native_state":             snapshot.NativeState,
		"native_last_status":       snapshot.NativeLastStatus,
	}
}
