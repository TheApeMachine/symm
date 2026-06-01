package public

import "encoding/json"

type Response struct {
	Error  []string `json:"error"`
	Result any      `json:"result"`
}

func (msg SocketMessage) SplitDataRows() ([]*SocketMessage, error) {
	var rows []json.RawMessage

	if err := json.Unmarshal(msg.Data, &rows); err != nil {
		return nil, err
	}

	result := make([]*SocketMessage, len(rows))

	for i, row := range rows {
		result[i] = &SocketMessage{
			Channel: msg.Channel,
			Type:    msg.Type,
			Data:    row,
		}
	}

	return result, nil
}
