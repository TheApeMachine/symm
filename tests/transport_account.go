package tests

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/tests/fixtures/balances"
)

func (transport *mockTransport) balance() []byte {
	transport.mu.RLock()
	configured := transport.balances
	transport.mu.RUnlock()

	if configured != nil {
		return envelope(configured)
	}

	fixture := balances.NewMarket("USD")
	var payload []byte

	for frame := range fixture.Generate() {
		payload = frame
	}

	var wsEnvelope map[string]any

	if err := json.Unmarshal(payload, &wsEnvelope); err != nil {
		panic(fmt.Errorf("tests: decode balance fixture: %w", err))
	}

	result := map[string]any{}
	data, _ := wsEnvelope["data"].([]any)

	for _, entry := range data {
		row, _ := entry.(map[string]any)
		asset, _ := row["asset"].(string)

		if asset != "" {
			result[asset] = fmt.Sprintf("%v", row["balance"])
		}
	}

	return envelope(result)
}

func (transport *mockTransport) accountBalances() map[string]float64 {
	response := struct {
		Result map[string]string `json:"result"`
	}{}

	if err := json.Unmarshal(transport.balance(), &response); err != nil {
		panic(fmt.Errorf("tests: decode account balances: %w", err))
	}

	account := make(map[string]float64, len(response.Result))

	for asset, value := range response.Result {
		balance, err := strconv.ParseFloat(value, 64)

		if err != nil {
			panic(fmt.Errorf("tests: decode %s account balance: %w", asset, err))
		}

		account[asset] = balance
	}

	return account
}

func (transport *mockTransport) setBalances(balances map[string]float64) {
	configured := make(map[string]string, len(balances))

	for asset, balance := range balances {
		configured[asset] = strconv.FormatFloat(balance, 'f', -1, 64)
	}

	transport.mu.Lock()
	transport.balances = configured
	transport.mu.Unlock()
}

func (transport *mockTransport) setPrice(pair string, price float64) {
	transport.mu.Lock()
	transport.prices[pair] = price
	transport.mu.Unlock()
}

func (transport *mockTransport) setBasis(pair string, cost float64) {
	transport.mu.Lock()
	transport.basis[pair] = cost
	transport.mu.Unlock()
}

func (transport *mockTransport) tradeBalance() []byte {
	account := transport.accountBalances()
	transport.mu.RLock()
	defer transport.mu.RUnlock()

	cash := 0.0
	marketValue := 0.0
	costBasis := 0.0
	seenQuotes := make(map[string]bool)

	for _, symbol := range transport.symbols {
		base, quote, known := splitPair(symbol.Pair)

		if !known {
			continue
		}

		if !seenQuotes[quote] {
			cash += account[quote]
			seenQuotes[quote] = true
		}

		price := transport.prices[symbol.Pair]

		if price == 0 {
			price = symbol.StartPrice
		}

		marketValue += account[base] * price
		costBasis += transport.basis[symbol.Pair]
	}

	tradeBalance := cash + costBasis
	unrealized := marketValue - costBasis
	equity := tradeBalance + unrealized

	return envelope(map[string]string{
		"eb":  strconv.FormatFloat(equity, 'f', -1, 64),
		"tb":  strconv.FormatFloat(tradeBalance, 'f', -1, 64),
		"m":   "0",
		"n":   strconv.FormatFloat(unrealized, 'f', -1, 64),
		"c":   strconv.FormatFloat(costBasis, 'f', -1, 64),
		"v":   strconv.FormatFloat(marketValue, 'f', -1, 64),
		"e":   strconv.FormatFloat(equity, 'f', -1, 64),
		"mf":  strconv.FormatFloat(equity, 'f', -1, 64),
		"mfo": strconv.FormatFloat(equity, 'f', -1, 64),
		"uv":  "0",
	})
}

func (transport *mockTransport) tradesHistory() []byte {
	transport.mu.RLock()
	configured := transport.trades
	transport.mu.RUnlock()

	if configured != nil {
		return envelope(map[string]any{
			"count": len(configured), "trades": configured,
		})
	}

	return envelope(map[string]any{
		"count": 0, "trades": map[string]any{},
	})
}

func (transport *mockTransport) openOrdersResponse() []byte {
	transport.mu.RLock()
	orders := transport.openOrders
	transport.mu.RUnlock()

	if orders == nil {
		orders = map[string]spot.Order{}
	}

	return envelope(map[string]any{"open": orders})
}

func (transport *mockTransport) tradeVolumeResponse() []byte {
	transport.mu.RLock()
	tradeVolume := transport.tradeVolume
	transport.mu.RUnlock()

	if tradeVolume == nil {
		return envelope(map[string]any{
			"currency": "ZUSD", "volume": "0",
		})
	}

	var payload []byte

	for frame := range tradeVolume.Generate() {
		payload = frame
	}

	return payload
}
