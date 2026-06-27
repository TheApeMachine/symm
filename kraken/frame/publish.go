package frame

import (
	"encoding/json"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

/*
Publish emits a Kraken private-channel frame to the UI bus and tree using the
same role, scope, and payload wrapping as the live private websocket.
*/
func Publish(
	tree *dmt.Tree,
	uiBroadcast *qpool.BroadcastGroup,
	rawPayload []byte,
	message *types.SocketMessage,
) error {
	output, outputErr := Artifact(rawPayload, message)
	if outputErr != nil {
		return errnie.Error(outputErr)
	}
	if output == nil {
		return nil
	}

	if tree != nil {
		tree.InsertArtifact(output.Prefix(), output)
	}

	return uiBroadcast.Send(output)
}

func Artifact(
	rawPayload []byte,
	message *types.SocketMessage,
) (*datura.Artifact, error) {
	if message == nil {
		return nil, nil
	}

	role := ChannelRole(rawPayload, message)

	if role == "" {
		return nil, nil
	}

	wrapped, wrapErr := WrapPayload(role, rawPayload, message)

	if wrapErr != nil {
		return nil, errnie.Error(wrapErr)
	}

	output := datura.Acquire("kraken:private", datura.APPJSON).
		WithDestination("ui").
		WithRole(role).
		WithPayload(wrapped)

	if message.Type != "" {
		output.WithScope(message.Type)
	}

	return output, nil
}

/*
AckOnlyRequest reports subscribe/unsubscribe requests that produce no outbound frame.
*/
func AckOnlyRequest(rawPayload []byte) bool {
	if len(rawPayload) == 0 {
		return false
	}

	var envelope struct {
		Method string `json:"method"`
	}

	if sonic.Unmarshal(rawPayload, &envelope) != nil {
		return false
	}

	switch envelope.Method {
	case "subscribe", "unsubscribe":
		return true
	default:
		return false
	}
}

/*
ChannelRole resolves the Kraken private channel name from a wire frame or message.
*/
func ChannelRole(rawPayload []byte, message *types.SocketMessage) string {
	if message != nil && strings.TrimSpace(message.Channel) != "" {
		return strings.TrimSpace(message.Channel)
	}

	if len(rawPayload) == 0 {
		return ""
	}

	var envelope struct {
		Channel string `json:"channel"`
		Result  struct {
			Channel string `json:"channel"`
		} `json:"result"`
		Params struct {
			Channel string `json:"channel"`
		} `json:"params"`
	}

	if sonic.Unmarshal(rawPayload, &envelope) != nil {
		return ""
	}

	if strings.TrimSpace(envelope.Channel) != "" {
		return strings.TrimSpace(envelope.Channel)
	}

	if strings.TrimSpace(envelope.Result.Channel) != "" {
		return strings.TrimSpace(envelope.Result.Channel)
	}

	return strings.TrimSpace(envelope.Params.Channel)
}

/*
WrapPayload shapes outbound UI payloads the same way live private websocket does.
*/
func WrapPayload(
	role string,
	rawPayload []byte,
	message *types.SocketMessage,
) ([]byte, error) {
	if message == nil || len(message.Data) == 0 {
		return rawPayload, nil
	}

	switch role {
	case "balances":
		return wrap("asset", message.Data)
	case "executions":
		return wrap("executions", message.Data)
	}

	if json.Valid(message.Data) && message.Data[0] == '{' {
		return message.Data, nil
	}

	return wrap("data", message.Data)
}

func wrap(key string, value json.RawMessage) ([]byte, error) {
	payload, err := sonic.Marshal(map[string]json.RawMessage{
		key: value,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "kraken/frame: marshal wrap", err))
	}

	return payload, nil
}
