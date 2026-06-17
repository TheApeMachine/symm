package ui

import (
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

/*
WalletFrame maps Kraken or paper balances into the dashboard wallet wire shape.
*/
func WalletFrame(balances *user.Balances) map[string]any {
	if balances == nil {
		return nil
	}

	quoteCurrency := strings.ToUpper(viper.GetString("market.quote_currency"))

	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}

	frame := map[string]any{
		"type":     "wallet",
		"Type":     "paper",
		"Currency": quoteCurrency,
	}

	if balances.Currency != "" {
		frame["Currency"] = balances.Currency
	}

	if logic.ScalarFinite(balances.Balance) && balances.Balance > 0 {
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

	if logic.ScalarFinite(balances.Realized) {
		frame["Realized"] = balances.Realized
	}

	if balances.Currency != "" ||
		(balances.Balance > 0 && logic.ScalarFinite(balances.Balance)) ||
		len(balances.Inventory) > 0 {
		if _, hasBalance := frame["Balance"]; !hasBalance && balances.Balance > 0 {
			frame["Balance"] = balances.Balance
		}

		return frame
	}

	inventory := make(map[string]float64)
	cashBalance := 0.0

	for _, assetRow := range balances.Asset {
		if assetRow.Asset == "" || !logic.ScalarFinite(assetRow.Balance) {
			continue
		}

		if strings.EqualFold(assetRow.Asset, quoteCurrency) {
			cashBalance += assetRow.Balance

			continue
		}

		if assetRow.Balance > 0 {
			inventory[strings.ToUpper(assetRow.Asset)] = assetRow.Balance
		}
	}

	frame["Balance"] = cashBalance

	if len(inventory) > 0 {
		frame["Inventory"] = inventory
	}

	return frame
}

/*
WalletFramePublishable reports whether a wallet frame should be sent to the dashboard.
*/
func WalletFramePublishable(frame map[string]any) bool {
	if frame == nil {
		return false
	}

	balance, ok := frame["Balance"].(float64)

	if ok && logic.ScalarFinite(balance) && balance > 0 {
		return true
	}

	inventory, ok := frame["Inventory"].(map[string]any)

	if ok && len(inventory) > 0 {
		return true
	}

	typedInventory, ok := frame["Inventory"].(map[string]float64)

	return ok && len(typedInventory) > 0
}

/*
PublishDecisionTree ships the embedded playbook branches to ui subscribers.
*/
func PublishDecisionTree(
	pool *qpool.Q[any],
	branches []*logic.Branch,
) error {
	if len(branches) == 0 {
		return errnie.Err(
			errnie.Validation,
			"hub: decision tree branches are empty",
			nil,
		)
	}

	return PublishPayload(pool, "decision_tree", map[string]any{
		"type":     "decision_tree",
		"branches": branches,
	})
}

/*
PublishWallet ships one wallet snapshot frame to ui subscribers.
*/
func PublishWallet(
	pool *qpool.Q[any],
	balances *user.Balances,
) error {
	frame := WalletFrame(balances)

	if !WalletFramePublishable(frame) {
		return nil
	}

	return PublishPayload(pool, "wallet", frame)
}

/*
PublishMark ships one live mark price for open-position pricing.
*/
func PublishMark(
	pool *qpool.Q[any],
	symbol string,
	price float64,
) error {
	if strings.TrimSpace(symbol) == "" || !logic.ScalarFinite(price) || price <= 0 {
		return nil
	}

	return PublishPayload(pool, "mark", map[string]any{
		"type":   "mark",
		"symbol": symbol,
		"price":  price,
	})
}

/*
PublishOhlc ships one anchor or position candle update to ui subscribers.
*/
func PublishOhlc(
	pool *qpool.Q[any],
	candle *krakenmarket.CandleUpdate,
) error {
	if candle == nil || strings.TrimSpace(candle.Symbol) == "" {
		return nil
	}

	sec, secErr := candleUnixSec(candle.IntervalBegin)

	if secErr != nil {
		return secErr
	}

	if !logic.ScalarFinite(candle.Open) ||
		!logic.ScalarFinite(candle.High) ||
		!logic.ScalarFinite(candle.Low) ||
		!logic.ScalarFinite(candle.Close) {
		return errnie.Err(
			errnie.Validation,
			"hub: ohlc candle fields are not finite",
			nil,
		)
	}

	volume := candle.Volume

	if !logic.ScalarFinite(volume) {
		volume = 0
	}

	return PublishPayload(pool, "ohlc", map[string]any{
		"type":   "ohlc",
		"symbol": candle.Symbol,
		"sec":    sec,
		"open":   candle.Open,
		"high":   candle.High,
		"low":    candle.Low,
		"close":  candle.Close,
		"volume": volume,
	})
}

func gaugeReadingsFromMeasurements(
	measurements []logic.Measurement,
) []map[string]any {
	latestBySource := make(map[logic.SourceType]logic.Measurement, logic.SourceCount)
	readingsCapacity := viper.GetInt("telemetry.gauge.readings_capacity")

	if readingsCapacity <= 0 {
		readingsCapacity = 1024
	}

	for _, measurement := range measurements {
		if !measurement.Publishable() {
			continue
		}

		existing, found := latestBySource[measurement.Source]

		if !found || measurement.ObservedAt.After(existing.ObservedAt) {
			latestBySource[measurement.Source] = measurement
		}
	}

	readings := make([]map[string]any, 0, len(latestBySource))

	for source, measurement := range latestBySource {
		readings = append(readings, map[string]any{
			"source":            string(source),
			"confidence":        measurement.Confidence,
			"surprise":          measurement.Surprise,
			"strength":          measurement.Strength,
			"elapsed":           measurement.Elapsed,
			"category":          string(measurement.Category),
			"observed_at":       measurement.ObservedAt,
			"calibrated":        true,
			"readings_capacity": readingsCapacity,
		})
	}

	return readings
}

func candleUnixSec(intervalBegin string) (int64, error) {
	trimmed := strings.TrimSpace(intervalBegin)

	if trimmed == "" {
		return 0, errnie.Err(
			errnie.Validation,
			"hub: ohlc interval_begin is empty",
			nil,
		)
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z",
	}

	for _, layout := range layouts {
		parsed, parseErr := time.Parse(layout, trimmed)

		if parseErr == nil {
			return parsed.Unix(), nil
		}
	}

	return 0, errnie.Err(
		errnie.Validation,
		"hub: ohlc interval_begin is not parseable",
		nil,
	)
}
