package mockapi

import (
	"encoding/json"

	"github.com/theapemachine/errnie"
)

/*
filterSymbols restricts a configured snapshot to the symbols in the request
while retaining the exact JSON numbers used by Kraken book checksums.
*/
func filterSymbols(
	channel string,
	payload []byte,
	symbols []string,
) ([]byte, bool, error) {
	frame := struct {
		Channel string          `json:"channel"`
		Type    string          `json:"type"`
		Data    json.RawMessage `json:"data"`
	}{}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, false, errnie.Err(errnie.Validation, "tests/mockapi: decode response", err)
	}

	if frame.Channel != channel || frame.Type == "" || len(frame.Data) == 0 {
		return nil, false, errnie.Err(
			errnie.Validation,
			"tests/mockapi: response envelope incomplete",
			nil,
		)
	}

	if len(symbols) == 0 {
		return append([]byte(nil), payload...), true, nil
	}

	rows := []json.RawMessage{}

	if err := json.Unmarshal(frame.Data, &rows); err != nil {
		return nil, false, errnie.Err(
			errnie.Validation,
			"tests/mockapi: symbol response data must be an array",
			err,
		)
	}

	requested := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		requested[symbol] = struct{}{}
	}

	filtered := make([]json.RawMessage, 0, len(rows))

	for _, row := range rows {
		identity := struct {
			Symbol string `json:"symbol"`
		}{}

		if err := json.Unmarshal(row, &identity); err != nil {
			return nil, false, errnie.Err(
				errnie.Validation,
				"tests/mockapi: filter symbol",
				err,
			)
		}

		if _, ok := requested[identity.Symbol]; ok {
			filtered = append(filtered, row)
		}
	}

	if len(filtered) == 0 {
		return nil, false, nil
	}

	encoded, err := json.Marshal(struct {
		Channel string            `json:"channel"`
		Type    string            `json:"type"`
		Data    []json.RawMessage `json:"data"`
	}{
		Channel: frame.Channel,
		Type:    frame.Type,
		Data:    filtered,
	})

	if err != nil {
		return nil, false, errnie.Err(errnie.Internal, "tests/mockapi: filter response", err)
	}

	return encoded, true, nil
}
