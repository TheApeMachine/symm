package response

import (
	"math"
	"strings"

	"github.com/bytedance/sonic"
)

func splitSymbol(symbol string) (string, string, bool) {
	base, quote, ok := strings.Cut(symbol, "/")
	if !ok {
		return "", "", false
	}

	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))

	return base, quote, base != "" && quote != ""
}

func (balances *Balances) adjustFee(asset string, fee float64) {
	if asset == "" || fee <= 0 {
		return
	}

	balances.adjustBalance(asset, -fee)
}

func (balances *Balances) adjustBalance(asset string, delta float64) {
	if balances == nil || balances.model == nil || asset == "" || delta == 0 {
		return
	}

	payload := balances.payload()
	rows, _ := payload["data"].([]any)
	index := balanceRowIndex(rows, asset)
	if index < 0 {
		rows = append(rows, newBalanceRow(asset))
		index = len(rows) - 1
	}

	row, _ := rows[index].(map[string]any)
	if row == nil {
		row = newBalanceRow(asset)
		rows[index] = row
	}

	current := floatValue(row["balance"])
	next := current + delta
	if math.Abs(next) < 1e-12 {
		next = 0
	}

	row["asset"] = strings.ToUpper(strings.TrimSpace(asset))
	row["asset_class"] = "currency"
	row["balance"] = next

	wallets, _ := row["wallets"].([]any)
	if len(wallets) == 0 {
		wallets = []any{map[string]any{
			"balance": 0.0,
			"type":    "spot",
			"id":      "main",
		}}
	}

	wallet, _ := wallets[0].(map[string]any)
	if wallet == nil {
		wallet = map[string]any{
			"type": "spot",
			"id":   "main",
		}
		wallets[0] = wallet
	}

	wallet["balance"] = next
	row["wallets"] = wallets
	payload["data"] = rows
	balances.storePayload(payload)
}

func balanceRowIndex(rows []any, asset string) int {
	target := strings.ToUpper(strings.TrimSpace(asset))

	for index, raw := range rows {
		row, _ := raw.(map[string]any)
		if row == nil {
			continue
		}

		current := strings.ToUpper(strings.TrimSpace(stringValue(row["asset"])))
		if current == target {
			return index
		}
	}

	return -1
}

func newBalanceRow(asset string) map[string]any {
	return map[string]any{
		"asset":       strings.ToUpper(strings.TrimSpace(asset)),
		"asset_class": "currency",
		"balance":     0.0,
		"wallets": []any{map[string]any{
			"balance": 0.0,
			"type":    "spot",
			"id":      "main",
		}},
	}
}

func (balances *Balances) payload() map[string]any {
	payload := make(map[string]any)
	if balances == nil || balances.model == nil {
		return payload
	}

	if err := sonic.Unmarshal(balances.model.DecryptPayload(), &payload); err != nil {
		return map[string]any{
			"channel": "balances",
			"type":    "snapshot",
			"data":    []any{},
		}
	}

	if _, ok := payload["data"].([]any); !ok {
		payload["data"] = []any{}
	}

	return payload
}

func (balances *Balances) storePayload(payload map[string]any) {
	wire, err := sonic.Marshal(payload)
	if err != nil {
		return
	}

	balances.model.WithPayload(wire)
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func stringValue(value any) string {
	typed, _ := value.(string)

	return typed
}
