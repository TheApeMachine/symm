package user

import (
	"encoding/json"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
)

/*
PublishRaw mirrors kraken/private publishRaw: one Kraken v2 channel frame on raw.
*/
func PublishRaw(
	raw *qpool.BroadcastGroup,
	channel string,
	msgType string,
	data json.RawMessage,
) {
	if raw == nil {
		return
	}

	raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": channel,
		"type":    msgType,
		"data":    append(json.RawMessage(nil), data...),
	}})
}

/*
PublishExecutionsRaw publishes one executions update frame and the derived trader
envelope for every row with a symbol — same split as the live private websocket.
*/
func PublishExecutionsRaw(raw *qpool.BroadcastGroup, msgType string, rows []Execution) {
	payload, err := sonic.Marshal(rows)

	if err != nil || raw == nil {
		return
	}

	PublishRaw(raw, "executions", msgType, payload)

	for _, execution := range rows {
		if execution.Symbol == "" || execution.ExecType != "trade" {
			continue
		}

		PublishExecutionDerived(raw, execution)
	}
}

/*
PublishExecutionDerived is the simplified execution map trader/crypto consumes.
*/
func PublishExecutionDerived(raw *qpool.BroadcastGroup, execution Execution) {
	if raw == nil {
		return
	}

	price := execution.LastPrice

	if price <= 0 {
		price = execution.AvgPrice
	}

	reason := execution.OrderStatus

	if reason == "" {
		reason = execution.ExecType
	}

	raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": "executions",
		"symbol":  execution.Symbol,
		"side":    execution.Side,
		"qty":     execution.LastQty,
		"price":   price,
		"fee":     ExecutionFeeTotal(execution),
		"reason":  reason,
	}})
}

/*
PublishExecutionRejectDerived clears an in-flight order without a Kraken trade row.
*/
func PublishExecutionRejectDerived(
	raw *qpool.BroadcastGroup,
	symbol, side, reason string,
) {
	if raw == nil {
		return
	}

	raw.Send(&qpool.QValue[any]{Value: map[string]any{
		"channel": "executions",
		"symbol":  symbol,
		"side":    side,
		"qty":     0.0,
		"price":   0.0,
		"fee":     0.0,
		"reason":  reason,
	}})
}

/*
ExecutionFeeTotal sums fee lines on a trade execution event.
*/
func ExecutionFeeTotal(execution Execution) float64 {
	fee := 0.0

	for _, row := range execution.Fees {
		fee += row.Qty
	}

	return fee
}

/*
PublishBalancesRaw publishes a balances channel frame on raw.
*/
func PublishBalancesRaw(
	raw *qpool.BroadcastGroup,
	msgType string,
	rows []Balance,
) {
	payload, err := sonic.Marshal(rows)

	if err != nil || raw == nil {
		return
	}

	PublishRaw(raw, "balances", msgType, payload)
}

/*
PublishWalletFromBalances updates the ui wallet from balances snapshot or update rows.
*/
func PublishWalletFromBalances(ui *qpool.BroadcastGroup, rows []Balance) {
	if ui == nil {
		return
	}

	quote := strings.ToUpper(viper.GetString("market.quote_currency"))

	for _, row := range rows {
		if !strings.EqualFold(row.Asset, quote) {
			continue
		}

		ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":   "wallet",
			"balance": row.Balance,
		}})

		return
	}
}
