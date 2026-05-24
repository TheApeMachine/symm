package ui

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
		}
	}

	return payload
}
