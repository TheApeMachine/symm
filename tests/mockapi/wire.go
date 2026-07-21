package mockapi

import (
	"encoding/json"

	"github.com/theapemachine/errnie"
)

/*
filterSymbols restricts a configured snapshot to the symbols in the request
while retaining the exact JSON numbers used by Kraken book checksums.
*/
func filterSymbols(payload []byte, symbols []string) []byte {
	if len(symbols) == 0 {
		return append([]byte(nil), payload...)
	}

	frame := struct {
		Channel string            `json:"channel"`
		Type    string            `json:"type"`
		Data    []json.RawMessage `json:"data"`
	}{}

	if err := json.Unmarshal(payload, &frame); err != nil || frame.Data == nil {
		return append([]byte(nil), payload...)
	}

	requested := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		requested[symbol] = struct{}{}
	}

	filtered := make([]json.RawMessage, 0, len(frame.Data))

	for _, row := range frame.Data {
		identity := struct {
			Symbol string `json:"symbol"`
		}{}

		if err := json.Unmarshal(row, &identity); err != nil {
			panic(errnie.Err(errnie.Validation, "tests/mockapi: filter symbol", err))
		}

		if _, ok := requested[identity.Symbol]; ok {
			filtered = append(filtered, row)
		}
	}

	frame.Data = filtered
	encoded, err := json.Marshal(frame)

	if err != nil {
		panic(errnie.Err(errnie.Internal, "tests/mockapi: filter response", err))
	}

	return encoded
}
