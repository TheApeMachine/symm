package ui

import (
	"time"

	"github.com/theapemachine/qpool"
)

/*
Publish sends one dashboard event on the shared ui broadcast group.
*/
func Publish(ui *qpool.BroadcastGroup, event string, payload map[string]any) {
	if ui == nil || payload == nil {
		return
	}

	payload["event"] = event
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	ui.Send(&qpool.QValue[any]{
		Value: omitEmptyCollections(payload),
	})
}

/*
SendEvent forwards a payload that already includes event metadata.
*/
func SendEvent(ui *qpool.BroadcastGroup, payload map[string]any) {
	if ui == nil || payload == nil {
		return
	}

	ui.Send(&qpool.QValue[any]{
		Value: omitEmptyCollections(payload),
	})
}

/*
omitEmptyCollections drops zero-length slices and maps from websocket payloads.
Empty arrays were being sent every rescore tick because per-tick buffers reset
before live track scores were merged into telemetry.
*/
func omitEmptyCollections(payload map[string]any) map[string]any {
	for key, value := range payload {
		switch typed := value.(type) {
		case []map[string]any:
			if len(typed) == 0 {
				delete(payload, key)
			}
		case []any:
			if len(typed) == 0 {
				delete(payload, key)
			}
		case map[string]any:
			if len(typed) == 0 {
				delete(payload, key)
			}
		case map[string]float64:
			if len(typed) == 0 {
				delete(payload, key)
			}
		case map[string]int:
			if len(typed) == 0 {
				delete(payload, key)
			}
		}
	}

	return payload
}
