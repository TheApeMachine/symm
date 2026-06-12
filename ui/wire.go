package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/user"
)

func uiWireFrame(message *qpool.QValue[any]) (map[string]any, error) {
	if message == nil {
		return nil, errnie.Error(errnie.Require(map[string]any{
			"message": message,
		}))
	}

	if message.Type == "balances" {
		return balanceWireFrame(message.Value)
	}

	if message.Type == "decision_tree" {
		return encodedWireFrame(message.Type, message.Value)
	}

	if frame, ok := message.Value.(map[string]any); ok {
		return mapWireFrame(message.Type, frame), nil
	}

	frame := map[string]any{}

	encoded, err := json.Marshal(message.Value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return map[string]any{
			"type":  message.Type,
			"value": message.Value,
		}, nil
	}

	return mapWireFrame(message.Type, frame), nil
}

func mapWireFrame(messageType string, frame map[string]any) map[string]any {
	wire := make(map[string]any, len(frame)+1)

	for key, value := range frame {
		wire[key] = value
	}

	if wireType, ok := wire["type"].(string); !ok || wireType == "" {
		wire["type"] = messageType
	}

	return wire
}

func encodedWireFrame(messageType string, value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)

	if err != nil {
		return nil, errnie.Error(err)
	}

	frame := map[string]any{}

	if err = json.Unmarshal(encoded, &frame); err != nil {
		return nil, errnie.Error(err)
	}

	return mapWireFrame(messageType, frame), nil
}

func balanceWireFrame(value any) (map[string]any, error) {
	balances, ok := value.(user.Balances)

	if !ok {
		return nil, errnie.Error(fmt.Errorf("ui: balances payload is %T", value))
	}

	quoteCurrency := viper.GetString("market.quote_currency")
	symbol := quoteCurrencySymbol(quoteCurrency)
	frame := map[string]any{
		"type": "balances",
		"assets": map[string]any{
			"asset": balances.Asset,
		},
		"balanceLabel":  "Balance",
		"openPositions": openPositionCount(balances, quoteCurrency),
		"symbol":        symbol,
	}
	attachBalanceMetadata(frame, balances)

	for _, asset := range balances.Asset {
		if !isQuoteAsset(asset.Asset, quoteCurrency) {
			continue
		}

		frame["balanceLabel"] = quoteBalanceLabel(
			asset.Balance,
			quoteCurrency,
			symbol,
		)
		frame["symbol"] = symbol

		return frame, nil
	}

	if len(balances.Asset) == 0 {
		return frame, nil
	}

	primary := balances.Asset[0]
	frame["balanceLabel"] = fmt.Sprintf("%.2f %s", primary.Balance, primary.Asset)
	frame["symbol"] = primary.Asset

	return frame, nil
}

func attachBalanceMetadata(frame map[string]any, balances user.Balances) {
	if balances.Currency != "" {
		frame["Currency"] = balances.Currency
	}

	if balances.Currency != "" || balances.Balance != 0 {
		frame["Balance"] = balances.Balance
	}

	if len(balances.Inventory) > 0 {
		frame["Inventory"] = balances.Inventory
	}

	if len(balances.AvgEntry) > 0 {
		frame["AvgEntry"] = balances.AvgEntry
	}

	if len(balances.Marks) > 0 {
		frame["Marks"] = balances.Marks
	}

	if len(balances.Expected) > 0 {
		frame["ExpectedExit"] = balances.Expected
	}

	if len(balances.Unrealized) > 0 {
		frame["Unrealized"] = balances.Unrealized
	}

	if len(balances.ExitFeeRate) > 0 {
		frame["ExitFeeRate"] = balances.ExitFeeRate
	}

	if balances.Realized != 0 {
		frame["Realized"] = balances.Realized
	}
}

func isQuoteAsset(asset string, quoteCurrency string) bool {
	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	if quote == "" {
		quote = "USD"
	}

	name := strings.ToUpper(strings.TrimSpace(asset))

	return name == quote || name == "Z"+quote
}

func quoteCurrencySymbol(quoteCurrency string) string {
	switch strings.ToUpper(strings.TrimSpace(quoteCurrency)) {
	case "EUR", "ZEUR":
		return "€"
	case "USD", "ZUSD", "":
		return "$"
	default:
		return strings.ToUpper(strings.TrimSpace(quoteCurrency))
	}
}

func quoteBalanceLabel(amount float64, quoteCurrency string, symbol string) string {
	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	if len(symbol) == 1 {
		return fmt.Sprintf("%s%.2f", symbol, amount)
	}

	return fmt.Sprintf("%.2f %s", amount, quote)
}

func openPositionCount(balances user.Balances, quoteCurrency string) int {
	quote := strings.ToUpper(strings.TrimSpace(quoteCurrency))

	if quote == "" {
		quote = "USD"
	}

	count := 0

	for _, asset := range balances.Asset {
		assetName := strings.ToUpper(strings.TrimSpace(asset.Asset))

		if asset.Balance <= 0 {
			continue
		}

		if assetName == quote || assetName == "Z"+quote {
			continue
		}

		count++
	}

	return count
}
