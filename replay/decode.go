package replay

import (
	"encoding/json"

	"github.com/bytedance/sonic"
)

type socketEnvelope struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type envelopeTyped interface {
	SetEnvelopeType(string)
}

/*
DecodeWSRows unmarshals one inbound WebSocket frame into row type T.
*/
func DecodeWSRows[T any](payload []byte) ([]T, string, error) {
	var message socketEnvelope

	if err := sonic.Unmarshal(payload, &message); err != nil {
		return nil, "", err
	}

	var rows []T

	if err := sonic.Unmarshal(message.Data, &rows); err != nil {
		return nil, message.Type, err
	}

	for index := range rows {
		if tagged, ok := any(&rows[index]).(envelopeTyped); ok {
			tagged.SetEnvelopeType(message.Type)
		}
	}

	return rows, message.Type, nil
}

/*
DecodeWSSnapshot unmarshals one inbound snapshot object frame into T.
*/
func DecodeWSSnapshot[T any](payload []byte) (*T, error) {
	var message socketEnvelope

	if err := sonic.Unmarshal(payload, &message); err != nil {
		return nil, err
	}

	var row T

	if err := sonic.Unmarshal(message.Data, &row); err != nil {
		return nil, err
	}

	return &row, nil
}
