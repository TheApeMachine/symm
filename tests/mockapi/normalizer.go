package mockapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
mockNormalizerClient gives Kraken's real symbol normalizer metadata for the
same simulated universe advertised by the instrument fixture.
*/
func mockNormalizerClient(symbols []string) *spot.WebSocket {
	client := spot.NewWebSocket()
	client.REST.Executor = func(request *http.Request) (*http.Response, error) {
		version := request.URL.Query().Get("assetVersion")
		body := `{"error":[],"result":{}}`

		if len(symbols) == 0 {
			body = normalizerDefaults(request.URL.Path, version)
		}

		if len(symbols) > 0 {
			result := normalizerResult(request.URL.Path, version, symbols)
			encoded, err := sonic.Marshal(map[string]any{
				"error":  []any{},
				"result": result,
			})

			if err != nil {
				return nil, err
			}

			body = string(encoded)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}

	return client
}

/*
normalizerDefaults retains the two decoder-test pairs used outside a market.
*/
func normalizerDefaults(path, version string) string {
	if path == "/0/public/Assets" && version == "1" {
		return `{"error":[],"result":{"BTC":{"altname":"XBT"},"ZEC":{"altname":"ZEC"},"USD":{"altname":"USD"}}}`
	}

	if path == "/0/public/Assets" {
		return `{"error":[],"result":{"XXBT":{"altname":"XBT"},"XZEC":{"altname":"ZEC"},"ZUSD":{"altname":"USD"}}}`
	}

	if path == "/0/public/AssetPairs" && version == "1" {
		return `{"error":[],"result":{"BTC/USD":{"wsname":"BTC/USD","base":"BTC","quote":"USD"},"ZEC/USD":{"wsname":"ZEC/USD","base":"ZEC","quote":"USD"}}}`
	}

	if path == "/0/public/AssetPairs" {
		return `{"error":[],"result":{"XXBTZUSD":{"wsname":"XBT/USD","base":"XXBT","quote":"ZUSD"},"XZECZUSD":{"wsname":"ZEC/USD","base":"XZEC","quote":"ZUSD"}}}`
	}

	return `{"error":[],"result":{}}`
}

/*
normalizerResult builds assets or pairs from the exact simulated symbols.
*/
func normalizerResult(path, version string, symbols []string) map[string]any {
	result := map[string]any{}

	for _, symbol := range symbols {
		parts := strings.Split(symbol, "/")

		if path == "/0/public/Assets" {
			result[parts[0]] = map[string]any{"altname": parts[0]}
			result[parts[1]] = map[string]any{"altname": parts[1]}
			continue
		}

		if path != "/0/public/AssetPairs" {
			continue
		}

		key := strings.ReplaceAll(symbol, "/", "")

		if version == "1" {
			key = symbol
		}

		result[key] = map[string]any{
			"wsname": symbol,
			"base":   parts[0],
			"quote":  parts[1],
		}
	}

	return result
}
