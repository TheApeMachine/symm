package broker

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type StoplossState uint8

const (
	UNKNOWN StoplossState = iota
	ARMED
	TRIGGERED
	EXIT_SUBMITTED
	EXIT_CONFIRMED
	EXIT_REJECTED
)

/*
Stoploss is the desk's protective exit for one open position. A zero quantity
means the position is closed. The stop trails the mark up and never loosens.
*/
type Stoploss struct {
	Symbol string
	State  StoplossState
	order  *datura.Artifact
}

/*
NewStoploss arms a trailing stop for a freshly opened long at the given mark. The
offset is the fraction below the peak the stop rides at (0.01 == 1%).
*/
func NewStoploss(order *datura.Artifact, symbol string) *Stoploss {
	if order == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"stoploss: nil order",
			nil,
		))
		return nil
	}

	price := stoplossEntryPrice(order)
	if price <= 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"stoploss: non-positive entry price for "+symbol,
			nil,
		))
		return nil
	}

	offset := stoplossOffset()
	stoploss := &Stoploss{
		Symbol: symbol,
		State:  ARMED,
		order:  order,
	}

	writeStoplossState(stoploss.order, map[string]any{
		"state":        int(stoploss.State),
		"state_label":  stoplossStateLabel(stoploss.State),
		"last_mark":    price,
		"recent_marks": []float64{price},
		"peak":         price,
		"stop":         price * (1 - offset),
		"offset":       offset,
		"side":         "sell",
		"retry_count":  0,
		"armed_at":     time.Now().UTC().Format(time.RFC3339Nano),
	})

	return stoploss
}

func stoplossEntryPrice(order *datura.Artifact) float64 {
	for _, path := range [][]any{
		{"params", "limit_price"},
		{"params", "price"},
		{"last_price"},
		{"avg_price"},
		{"price"},
		{"entry_price"},
		{"data", 0, "last_price"},
		{"data", 0, "avg_price"},
		{"data", 0, "price"},
	} {
		price := datura.Peek[float64](order, path...)
		if price > 0 {
			return price
		}
	}

	return 0
}

func stoplossOffset() float64 {
	offset := viper.GetFloat64("trading.stop.trailing_offset_bps") / 10000.0

	if offset <= 0 {
		offset = 0.01
	}

	return offset
}

/*
Ratchet raises the trailing stop as the mark makes new highs and reports whether
the mark has fallen through the stop, in which case the position must exit.
*/
func (stoploss *Stoploss) Ratchet(mark float64) *Stoploss {
	if stoploss == nil || stoploss.order == nil || mark <= 0 {
		return stoploss
	}

	state := stoplossState(stoploss.order)
	if state == nil {
		return stoploss
	}

	peak, _ := state["peak"].(float64)
	stop, _ := state["stop"].(float64)
	offset, _ := state["offset"].(float64)
	exitPending := stoploss.State == EXIT_SUBMITTED ||
		stoploss.State == EXIT_CONFIRMED ||
		stoploss.State == EXIT_REJECTED

	if stop <= 0 && offset > 0 {
		stop = peak * (1 - offset)
	}

	if mark > peak {
		peak = mark

		if raised := peak * (1 - offset); raised > stop {
			stop = raised
		}
	}

	state["last_mark"] = mark
	state["recent_marks"] = appendStopMark(state["recent_marks"], mark)
	state["peak"] = peak
	state["stop"] = stop
	state["state"] = int(stoploss.State)
	state["state_label"] = stoplossStateLabel(stoploss.State)

	if !exitPending && mark <= stop {
		stoploss.State = TRIGGERED
		state["state"] = int(TRIGGERED)
		state["state_label"] = stoplossStateLabel(TRIGGERED)
		state["trigger"] = mark
		state["triggered_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	writeStoplossState(stoploss.order, state)

	return stoploss
}

func stoplossState(order *datura.Artifact) map[string]any {
	if order == nil {
		return nil
	}

	attributes, err := order.Attributes()
	if err != nil || len(attributes) == 0 {
		return nil
	}

	var root map[string]any
	if err := sonic.Unmarshal(attributes, &root); err != nil {
		return nil
	}

	state, _ := root["stoploss"].(map[string]any)
	return state
}

func writeStoplossState(order *datura.Artifact, state map[string]any) {
	if order == nil || state == nil {
		return
	}

	root := make(map[string]any)
	if attributes, err := order.Attributes(); err == nil && len(attributes) > 0 {
		_ = sonic.Unmarshal(attributes, &root)
	}

	root["stoploss"] = state

	wire, err := sonic.Marshal(root)
	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "stoploss: marshal state", err))
		return
	}

	errnie.Error(order.SetAttributes(wire))
}

func appendStopMark(marks any, mark float64) []float64 {
	const capMarks = 64
	trim := func(values []float64) []float64 {
		if len(values) <= capMarks {
			return values
		}

		return values[len(values)-capMarks:]
	}

	switch value := marks.(type) {
	case []float64:
		return trim(append(value, mark))
	case []any:
		out := make([]float64, 0, len(value)+1)
		for _, raw := range value {
			if parsed, ok := raw.(float64); ok {
				out = append(out, parsed)
			}
		}
		return trim(append(out, mark))
	default:
		return []float64{mark}
	}
}

func (stoploss *Stoploss) Close() {
	if stoploss.order != nil {
		stoploss.order = nil
	}

	stoploss.State = UNKNOWN
}

func stoplossStateLabel(state StoplossState) string {
	switch state {
	case ARMED:
		return "ARMED"
	case TRIGGERED:
		return "TRIGGERED"
	case EXIT_SUBMITTED:
		return "EXIT_SUBMITTED"
	case EXIT_CONFIRMED:
		return "EXIT_CONFIRMED"
	case EXIT_REJECTED:
		return "EXIT_REJECTED"
	default:
		return "UNKNOWN"
	}
}
