package public

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
)

type emulatorAckRequest struct {
	Method string         `json:"method"`
	ReqID  int64          `json:"req_id,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

func emulatorRequestAck(
	requestWire []byte,
	event *datura.Artifact,
	received time.Time,
	missingHandler bool,
) []byte {
	request := emulatorAckRequest{}
	_ = sonic.Unmarshal(requestWire, &request)

	method := strings.TrimSpace(request.Method)
	if method == "" {
		method = "unknown"
	}

	if received.IsZero() {
		received = time.Now().UTC()
	}

	out := map[string]any{
		"method":   method,
		"success":  true,
		"time_in":  received.UTC().Format(time.RFC3339Nano),
		"time_out": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if request.ReqID != 0 {
		out["req_id"] = request.ReqID
	}

	if method == "ping" {
		out["method"] = "pong"
		return marshalEmulatorAck(out)
	}

	if missingHandler {
		out["success"] = false
		out["error"] = "EGeneral:Invalid arguments:unknown channel"
		return marshalEmulatorAck(out)
	}

	if reason := emulatorRejectReason(event); reason != "" {
		out["success"] = false
		out["error"] = reason
		return marshalEmulatorAck(out)
	}

	if result := emulatorAckResult(request, event); len(result) > 0 {
		out["result"] = result
	}

	return marshalEmulatorAck(out)
}

func marshalEmulatorAck(out map[string]any) []byte {
	wire, err := sonic.Marshal(out)
	if err != nil {
		return []byte(`{"method":"unknown","success":false,"error":"EGeneral:Internal error"}`)
	}

	return wire
}

func emulatorRejectReason(event *datura.Artifact) string {
	row := emulatorEventRow(event)
	if row == nil {
		return ""
	}

	status := strings.ToLower(strings.TrimSpace(emulatorStringValue(row["order_status"])))
	if status != "rejected" {
		return ""
	}

	return emulatorStringValue(row["reject_reason"])
}

func emulatorAckResult(request emulatorAckRequest, event *datura.Artifact) map[string]any {
	result := map[string]any{}

	switch request.Method {
	case "subscribe", "unsubscribe":
		if channel := emulatorStringValue(request.Params["channel"]); channel != "" {
			result["channel"] = channel
		}
	case "add_order", "amend_order", "edit_order", "cancel_order":
		row := emulatorEventRow(event)
		for _, key := range []string{"order_id", "cl_ord_id", "order_userref"} {
			if value := row[key]; value != nil && value != "" {
				result[key] = value
			}
		}
	}

	return result
}

func emulatorEventRow(event *datura.Artifact) map[string]any {
	if event == nil {
		return nil
	}

	payload := map[string]any{}
	if err := sonic.Unmarshal(event.DecryptPayload(), &payload); err != nil {
		return nil
	}

	data, _ := payload["data"].([]any)
	if len(data) == 0 {
		return nil
	}

	row, _ := data[0].(map[string]any)
	return row
}

func emulatorStringValue(value any) string {
	typed, _ := value.(string)
	return typed
}
