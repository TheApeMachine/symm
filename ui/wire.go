package ui

import (
	"encoding/json"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/user"
)

func uiWireFrame(message *qpool.QValue[any]) (map[string]any, error) {
	if message == nil {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"message": message,
		}))
	}

	if message.Type == "balances" {
		return balanceWireFrame(message.Value)
	}

	if message.Type == "decision_tree" {
		return encodedWireFrame(message.Type, message.Value)
	}

	if frame, ok := message.Value.(map[string]any); ok {
		return mapWireFrame(message.Type, frame), nil
	}

	frame := map[string]any{}

	encoded, err := json.Marshal(message.Value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return map[string]any{
			"type":  message.Type,
			"value": message.Value,
		}, nil
	}

	return mapWireFrame(message.Type, frame), nil
}

func mapWireFrame(messageType string, frame map[string]any) map[string]any {
	wire := make(map[string]any, len(frame)+1)

	for key, value := range frame {
		wire[key] = value
	}

	if wireType, ok := wire["type"].(string); !ok || wireType == "" {
		wire["type"] = messageType
	}

	return wire
}

func encodedWireFrame(messageType string, value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	frame := map[string]any{}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return nil, errnie.Error(err)
	}

	return mapWireFrame(messageType, frame), nil
}

func balanceWireFrame(value any) (map[string]any, error) {
	balances, ok := value.(user.Balances)

	if !ok {
		return nil, errnie.Error(fmt.Errorf("ui: balances payload is %T", value))
	}

	frame := map[string]any{
		"type": "balances",
		"assets": map[string]any{
			"asset": balances.Asset,
		},
		"balanceLabel": "Balance",
		"symbol":       "$",
	}

	for _, asset := range balances.Asset {
		if asset.Asset != "ZUSD" && asset.Asset != "USD" {
			continue
		}

		frame["balanceLabel"] = fmt.Sprintf("$%.2f", asset.Balance)
		frame["symbol"] = "$"

		return frame, nil
	}

	if len(balances.Asset) == 0 {
		return frame, nil
	}

	primary := balances.Asset[0]
	frame["balanceLabel"] = fmt.Sprintf("%.2f %s", primary.Balance, primary.Asset)
	frame["symbol"] = primary.Asset

	return frame, nil
}
