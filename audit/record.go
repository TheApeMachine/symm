package audit

/*
Record writes one structured diagnostic row to the audit jsonl file.
Diagnostic rows use channel "diagnostic" so they can be filtered apart from
bus traffic mirrors (measurements, raw, ui).
*/
func Record(recorder *Recorder, eventType string, value any) error {
	if recorder == nil {
		return nil
	}

	return recorder.Write(map[string]any{
		"channel": "diagnostic",
		"type":    eventType,
		"value":   value,
	})
}
