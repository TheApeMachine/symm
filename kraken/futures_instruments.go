package kraken

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const futuresInstrumentsURL = "https://futures.kraken.com/derivatives/api/v3/instruments"

/*
FuturesInstrument is one tradeable (or listed) contract from the Futures REST
instruments endpoint. Symbol is the product_id used on the WebSocket feeds.
*/
type FuturesInstrument struct {
	Symbol    string `json:"symbol"`
	Type      string `json:"type"`
	Tradeable bool   `json:"tradeable"`
	Base      string `json:"base"`
	Quote     string `json:"quote"`
	Pair      string `json:"pair"`
}

type futuresInstrumentsResponse struct {
	Result      string              `json:"result"`
	Instruments []FuturesInstrument `json:"instruments"`
}

/*
FetchFuturesInstruments loads the public Futures instrument catalog.
*/
func FetchFuturesInstruments(ctx context.Context) ([]FuturesInstrument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, futuresInstrumentsURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("futures instruments: HTTP %d", res.StatusCode)
	}

	var parsed futuresInstrumentsResponse
	if err := sonic.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	if parsed.Result != "" && !strings.EqualFold(parsed.Result, "success") {
		return nil, fmt.Errorf("futures instruments: result %q", parsed.Result)
	}

	return parsed.Instruments, nil
}

/*
SpotSymbolForFutures maps a Futures instrument onto a spot WS symbol
(e.g. BTC/USD) using base/quote when present, else the pair field (BTC:USD).
*/
func SpotSymbolForFutures(instrument FuturesInstrument) string {
	base := strings.TrimSpace(instrument.Base)
	quote := strings.TrimSpace(instrument.Quote)

	if base != "" && quote != "" {
		return base + "/" + quote
	}

	pair := strings.TrimSpace(instrument.Pair)
	if pair == "" {
		return ""
	}

	pair = strings.ReplaceAll(pair, ":", "/")
	return pair
}

/*
PreferPerpetual picks one tradeable perpetual product_id for a spot symbol.
PF_ (flexible) wins over PI_ (inverse) when both exist for the same base/quote.
*/
func PreferPerpetual(candidates []FuturesInstrument) string {
	var pf, pi, other string

	for _, candidate := range candidates {
		if !candidate.Tradeable || candidate.Symbol == "" {
			continue
		}

		symbol := candidate.Symbol
		switch {
		case strings.HasPrefix(symbol, "PF_"):
			pf = symbol
		case strings.HasPrefix(symbol, "PI_"):
			pi = symbol
		default:
			if other == "" {
				other = symbol
			}
		}
	}

	if pf != "" {
		return pf
	}
	if pi != "" {
		return pi
	}
	return other
}
