package public

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

const pairOnline = "online"

type assetPairRow struct {
	Wsname string `json:"wsname"`
	Status string `json:"status"`
}

type assetPairsBody struct {
	Error  []string                  `json:"error"`
	Result map[string]assetPairRow   `json:"result"`
}

/*
AssetPairsTreePrefix returns the tree seek prefix for AssetPairs REST rows.
*/
func AssetPairsTreePrefix() []byte {
	return []byte(string(EndpointTypeAssetPairs))
}

func assetPairsRequest() *datura.Artifact {
	request := datura.Acquire("public", datura.APPJSON)

	_ = request.SetMetaValues(map[string]any{
		"method":      "GET",
		"destination": string(EndpointTypeAssetPairs),
		"headers":     map[string]string{},
	})

	return request
}

/*
DiscoverSymbols issues a REST AssetPairs artifact request and returns filtered symbols.
*/
func DiscoverSymbols(ctx context.Context, rest *Rest) ([]string, error) {
	if rest == nil {
		return nil, errnie.Error(fmt.Errorf("kraken/public: nil rest client"))
	}

	request := assetPairsRequest()

	defer request.Release()

	response := rest.Do(ctx, request)

	if response == nil {
		return nil, errnie.Error(fmt.Errorf("kraken/public: asset pairs response nil"))
	}

	defer response.Release()

	if response.HasError() {
		_, artifactErr := response.Error()

		return nil, errnie.Error(artifactErr)
	}

	payload, payloadErr := response.Payload()

	if payloadErr != nil {
		return nil, errnie.Error(payloadErr)
	}

	quote := strings.ToUpper(viper.GetString("market.quote_currency"))
	maxScan := viper.GetInt("market.max_scan_symbols")
	defaults := viper.GetStringSlice("market.default_symbols")

	return FilterQuoteSymbols(payload, quote, maxScan, defaults), nil
}

/*
FilterQuoteSymbols keeps online pairs in quote, caps scan size, and always includes defaults.
*/
func FilterQuoteSymbols(
	payload []byte,
	quote string,
	maxScan int,
	defaults []string,
) []string {
	if len(payload) == 0 {
		return copySymbols(defaults)
	}

	body := assetPairsBody{}

	if err := sonic.Unmarshal(payload, &body); err != nil {
		return copySymbols(defaults)
	}

	if len(body.Error) > 0 {
		return copySymbols(defaults)
	}

	matched := matchedQuoteSymbols(body.Result, quote)
	selected := selectSymbols(matched, maxScan, defaults)

	return selected
}

func matchedQuoteSymbols(pairs map[string]assetPairRow, quote string) []string {
	if len(pairs) == 0 {
		return nil
	}

	quote = strings.ToUpper(strings.TrimSpace(quote))
	suffix := "/" + quote
	symbols := make([]string, 0, len(pairs))

	for _, pair := range pairs {
		if pair.Status != pairOnline || pair.Wsname == "" {
			continue
		}

		symbol := normalizeWSSymbol(pair.Wsname)

		if quote != "" && !strings.HasSuffix(symbol, suffix) {
			continue
		}

		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}

func selectSymbols(matched []string, maxScan int, defaults []string) []string {
	if maxScan <= 0 {
		maxScan = len(matched)
	}

	selected := make([]string, 0, maxScan)
	seen := make(map[string]struct{}, maxScan)

	for _, symbol := range defaults {
		trimmed := strings.TrimSpace(symbol)

		if trimmed == "" {
			continue
		}

		normalized := normalizeWSSymbol(trimmed)

		if _, ok := seen[normalized]; ok {
			continue
		}

		seen[normalized] = struct{}{}
		selected = append(selected, normalized)
	}

	for _, symbol := range matched {
		if _, ok := seen[symbol]; ok {
			continue
		}

		if len(selected) >= maxScan {
			break
		}

		seen[symbol] = struct{}{}
		selected = append(selected, symbol)
	}

	return selected
}

func normalizeWSSymbol(symbol string) string {
	return strings.Replace(symbol, "XBT/", "BTC/", 1)
}

func copySymbols(symbols []string) []string {
	if len(symbols) == 0 {
		return nil
	}

	copied := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		trimmed := strings.TrimSpace(symbol)

		if trimmed == "" {
			continue
		}

		copied = append(copied, normalizeWSSymbol(trimmed))
	}

	return copied
}
