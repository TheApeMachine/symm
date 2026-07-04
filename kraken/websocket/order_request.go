package websocket

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type OrderRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
	ReqID  int64          `json:"req_id"`
}

func NewOrderRequest(artifact *datura.Artifact) (*OrderRequest, error) {
	if artifact == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: nil private request",
			nil,
		))
	}

	var request OrderRequest
	if err := sonic.Unmarshal(artifact.DecryptPayload(), &request); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: decode private request",
			err,
		))
	}

	request.Method = strings.TrimSpace(request.Method)
	if request.Method == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: private request method required",
			nil,
		))
	}

	if len(request.Params) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"account: private request params required",
			nil,
		))
	}

	return &request, nil
}

func (request *OrderRequest) String(key string) string {
	if request.Params[key] == nil {
		return ""
	}

	return strings.TrimSpace(fmt.Sprint(request.Params[key]))
}

func (request *OrderRequest) Float(key string) (float64, error) {
	value := request.Params[key]
	switch typed := value.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case string:
		out, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil {
			return out, nil
		}
	}

	return 0, errnie.Error(errnie.Err(
		errnie.Validation,
		"account: numeric private request param required: "+key,
		nil,
	))
}
