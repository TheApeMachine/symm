package rest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/symm/kraken/core"
)

const balancePath = "/0/private/Balance"

/*
Balance holds spot wallet balances keyed by Kraken asset code.
*/
type Balance struct {
	Error  []string          `json:"error"`
	Result map[string]string `json:"result"`
}

/*
FetchBalance loads spot balances for one API key pair.
*/
func FetchBalance(publicKey, privateKey string) (*Balance, error) {
	nonce := fmt.Sprint(time.Now().UnixMilli())
	bodyMap := map[string]any{"nonce": nonce}

	bodyBytes, err := json.Marshal(bodyMap)

	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	bodyString := string(bodyBytes)
	signature, err := signRequest(privateKey, bodyString, nonce, balancePath)

	if err != nil {
		return nil, fmt.Errorf("sign balance request: %w", err)
	}

	response, err := client.Post(strings.Join(
		[]string{core.KRAKEN_API_URL, balancePath}, "",
	), client.Config{
		Body: bodyBytes,
		Header: map[string]string{
			"Content-Type": "application/json",
			"API-Key":      publicKey,
			"API-Sign":     signature,
		},
	})

	if err != nil {
		return nil, err
	}

	defer response.Close()

	var balance Balance

	if err = json.Unmarshal(response.Body(), &balance); err != nil {
		return nil, err
	}

	if len(balance.Error) > 0 {
		return nil, fmt.Errorf("kraken balance error: %v", balance.Error)
	}

	return &balance, nil
}

/*
QuoteBalance returns the configured quote currency balance when present.
*/
func (balance *Balance) QuoteBalance(quoteCurrency string) (float64, bool) {
	if balance == nil || len(balance.Result) == 0 {
		return 0, false
	}

	raw, ok := balance.Result[strings.ToUpper(quoteCurrency)]

	if !ok {
		raw, ok = balance.Result["Z"+strings.ToUpper(quoteCurrency)]
	}

	if !ok || strings.TrimSpace(raw) == "" {
		return 0, false
	}

	var amount float64

	if _, err := fmt.Sscan(raw, &amount); err != nil {
		return 0, false
	}

	return amount, true
}
