package audit

import (
	"fmt"
)

/*
Record validates and writes one typed analytical event.
*/
func Record(recorder *Recorder, event any) error {
	if recorder == nil {
		return nil
	}

	if event == nil {
		return fmt.Errorf("audit: event required")
	}

	return recorder.Write(map[string]any{
		"channel": "analysis",
		"type":    fmt.Sprintf("%T", event),
		"value":   event,
	})
}
