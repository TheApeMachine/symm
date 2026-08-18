package audit

import (
	"fmt"

	"github.com/bytedance/sonic"
)

/*
Record validates and writes one typed analytical event: the curated audit
stream of decision moments. A recorder with an event sink writes there —
the sqlite audit tables — and never touches the file.
*/
func Record(recorder *Recorder, event any) error {
	if recorder == nil {
		return nil
	}

	if event == nil {
		return fmt.Errorf("audit: event required")
	}

	if recorder.EventSink != nil {
		payload, err := sonic.Marshal(event)

		if err != nil {
			return fmt.Errorf("audit: encode event: %w", err)
		}

		return recorder.EventSink(fmt.Sprintf("%T", event), payload)
	}

	return recorder.Write(map[string]any{
		"channel": "analysis",
		"type":    fmt.Sprintf("%T", event),
		"value":   event,
	})
}
