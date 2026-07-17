package tests

import (
	"iter"
	"time"

	"github.com/bytedance/sonic"
)

/*
Repeat expands a short fixture timeline to horizon frames by cycling payloads
and advancing timestamps — used when SNAPSHOT books must warm exhaust/depthflow
across many Cuts.
*/
func Repeat(frames iter.Seq[Frame], horizon int) iter.Seq[Frame] {
	collected := make([]Frame, 0)

	for frame := range frames {
		collected = append(collected, frame)
	}

	if len(collected) == 0 || horizon < 1 {
		return func(func(Frame) bool) {}
	}

	return func(yield func(Frame) bool) {
		for index := 0; index < horizon; index++ {
			frame := stampFrame(collected[index%len(collected)], index)

			if !yield(frame) {
				return
			}
		}
	}
}

func stampFrame(frame Frame, index int) Frame {
	var payload map[string]any

	if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
		panic(err)
	}

	for _, row := range frameRows(payload) {
		if stamp, ok := row["timestamp"].(string); ok {
			row["timestamp"] = advanceTimestamp(
				stamp, time.Duration(index)*250*time.Millisecond,
			)
		}
	}

	if stamp, ok := payload["timestamp"].(string); ok {
		payload["timestamp"] = advanceTimestamp(
			stamp, time.Duration(index)*250*time.Millisecond,
		)
	}

	return Frame{
		Channel: frame.Channel,
		Type:    frame.Type,
		Payload: marshalFrame(payload),
	}
}
