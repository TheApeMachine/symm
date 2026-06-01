package public

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bytedance/sonic"
)

const MethodSubscribe = "subscribe"

type SocketMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

/*
SplitDataRows expands one envelope into per-row messages, preserving channel and type.
Non-array data is returned as a single row unchanged.
*/
func (message *SocketMessage) SplitDataRows() ([]*SocketMessage, error) {
	if message == nil || len(message.Data) == 0 {
		return nil, nil
	}

	var rows []json.RawMessage

	if err := sonic.Unmarshal(message.Data, &rows); err != nil {
		row := *message

		return []*SocketMessage{&row}, nil
	}

	messages := make([]*SocketMessage, len(rows))

	for index := range rows {
		messages[index] = &SocketMessage{
			Channel: message.Channel,
			Type:    message.Type,
			Data:    rows[index],
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("kraken ws decode %s: empty data array", message.Channel)
	}

	return messages, nil
}

/*
EmitRows sends each split row to out, stopping when ctx is canceled.
*/
func (message *SocketMessage) EmitRows(
	ctx context.Context, out chan<- *SocketMessage,
) error {
	rows, err := message.SplitDataRows()

	if err != nil {
		return err
	}

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- row:
		}
	}

	return nil
}

/*
Subscription is the Kraken WebSocket v2 control frame: a method ("subscribe") and
the channel-specific params payload sent to open a feed.
*/
type Subscription struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}
