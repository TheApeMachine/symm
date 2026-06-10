package ui

import (
	"fmt"
)

/*
coalesceKey identifies the latest-wins slot for high-frequency ui telemetry.
Unique keys preserve every frame (decisions, snapshots).
*/
func coalesceKey(messageType string, value any) string {
	if messageType == "wallet" {
		return "wallet"
	}

	payload, ok := value.(map[string]any)

	if !ok {
		return messageType
	}

	switch messageType {
	case "gauge":
		return fmt.Sprintf("gauge:%s", mapString(payload, "source"))
	case "mark":
		return fmt.Sprintf("mark:%s", mapString(payload, "symbol"))
	case "ohlc":
		if symbol := mapString(payload, "symbol"); symbol != "" {
			return fmt.Sprintf("ohlc:%s", symbol)
		}
	case "prediction":
		kind := mapString(payload, "kind")

		if kind != "" {
			return fmt.Sprintf("prediction:%s", kind)
		}
	case "decision":
		if symbol := mapString(payload, "symbol"); symbol != "" {
			return fmt.Sprintf("decision:%s", symbol)
		}
	}

	return messageType
}

func mapString(payload map[string]any, field string) string {
	raw, ok := payload[field]

	if !ok || raw == nil {
		return ""
	}

	text, ok := raw.(string)

	if !ok {
		return fmt.Sprint(raw)
	}

	return text
}
