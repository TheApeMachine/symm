package ui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
WalletFrame maps a balances artifact payload into the dashboard wallet wire shape.
*/
func WalletFrame(artifact *datura.Artifact) map[string]any {
	if artifact == nil {
		return nil
	}

	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return nil
	}

	var wire map[string]any

	if json.Unmarshal(payload, &wire) != nil {
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

	if currency, ok := wire["Currency"].(string); ok && currency != "" {
		frame["Currency"] = currency
	}

	if currency, ok := wire["currency"].(string); ok && currency != "" {
		frame["Currency"] = currency
	}

	if balance, ok := wire["Balance"].(float64); ok && logic.ScalarFinite(balance) && balance > 0 {
		frame["Balance"] = balance
	}

	if balance, ok := wire["balance"].(float64); ok && logic.ScalarFinite(balance) && balance > 0 {
		frame["Balance"] = balance
	}

	if inventory, ok := wire["Inventory"].(map[string]any); ok && len(inventory) > 0 {
		frame["Inventory"] = inventory
	}

	if inventory, ok := wire["inventory"].(map[string]float64); ok && len(inventory) > 0 {
		frame["Inventory"] = inventory
	}

	if len(frame) > 3 {
		return frame
	}

	inventory := make(map[string]float64)
	cashBalance := 0.0
	rows, _ := wire["asset"].([]any)

	for _, rowAny := range rows {
		row, ok := rowAny.(map[string]any)

		if !ok {
			continue
		}

		asset, _ := row["asset"].(string)
		balance, _ := row["balance"].(float64)

		if asset == "" || !logic.ScalarFinite(balance) {
			continue
		}

		if strings.EqualFold(asset, quoteCurrency) {
			cashBalance += balance

			continue
		}

		if balance > 0 {
			inventory[strings.ToUpper(asset)] = balance
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
	artifact *datura.Artifact,
) error {
	frame := WalletFrame(artifact)

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
	artifact *datura.Artifact,
) error {
	if artifact == nil {
		return nil
	}

	payload, payloadOK := artifact.PayloadQuiet()

	if !payloadOK {
		return errnie.Err(errnie.Validation, "hub: ohlc payload missing", nil)
	}

	var wire map[string]any

	if json.Unmarshal(payload, &wire) != nil {
		return errnie.Err(errnie.Validation, "hub: ohlc payload invalid", nil)
	}

	symbol, _ := wire["symbol"].(string)

	if strings.TrimSpace(symbol) == "" {
		return nil
	}

	intervalBegin, _ := wire["interval_begin"].(string)
	sec, secErr := candleUnixSec(intervalBegin)

	if secErr != nil {
		return secErr
	}

	open, _ := wire["open"].(float64)
	high, _ := wire["high"].(float64)
	low, _ := wire["low"].(float64)
	closePrice, _ := wire["close"].(float64)

	if !logic.ScalarFinite(open) ||
		!logic.ScalarFinite(high) ||
		!logic.ScalarFinite(low) ||
		!logic.ScalarFinite(closePrice) {
		return errnie.Err(
			errnie.Validation,
			"hub: ohlc candle fields are not finite",
			nil,
		)
	}

	volume, _ := wire["volume"].(float64)

	if !logic.ScalarFinite(volume) {
		volume = 0
	}

	return PublishPayload(pool, "ohlc", map[string]any{
		"type":   "ohlc",
		"symbol": symbol,
		"sec":    sec,
		"open":   open,
		"high":   high,
		"low":    low,
		"close":  closePrice,
		"volume": volume,
	})
}

/*
StateFrame builds the dashboard heartbeat payload from live story measurements.
*/
func StateFrame(
	measurements []logic.Measurement,
	storyTicks uint64,
	playbookEvaluations int,
	walk logic.WalkTrace,
) map[string]any {
	frame := map[string]any{
		"type":                 "state",
		"story_ticks":          storyTicks,
		"measurements":         measurements,
		"gauge_readings":       gaugeReadingsFromMeasurements(measurements),
		"playbook_evaluations": playbookEvaluations,
	}

	if len(walk.Steps) > 0 {
		frame["decision_walk"] = walk
	}

	return frame
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
