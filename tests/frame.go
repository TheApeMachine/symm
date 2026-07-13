package tests

import (
	"math"
	"time"

	"github.com/bytedance/sonic"
)

var priceFields = map[string][]string{
	"ticker": {"last", "bid", "ask", "high", "low", "change", "change_pct"},
	"trade":  {"price"},
	"book":   {"price"},
}

var volumeFields = map[string][]string{
	"ticker": {"volume", "bid_qty", "ask_qty"},
	"trade":  {"qty"},
	"book":   {"qty"},
}

func cloneFrame(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(err)
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(err)
	}

	return out
}

func frameChannel(frame map[string]any) string {
	channel, _ := frame["channel"].(string)

	return channel
}

func frameRows(frame map[string]any) []map[string]any {
	data, ok := frame["data"].([]any)

	if !ok {
		return nil
	}

	rows := make([]map[string]any, 0, len(data))

	for _, item := range data {
		row, ok := item.(map[string]any)

		if !ok {
			continue
		}

		rows = append(rows, row)
	}

	return rows
}

func roundField(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func advanceTimestamp(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(err)
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func scaleFrameFields(
	frame map[string]any,
	priceMul float64,
	volumeMul float64,
	volumeAdd float64,
	index int,
) {
	channel := frameChannel(frame)

	for _, row := range frameRows(frame) {
		scaleRowFields(row, priceMul, priceFields[channel])
		addRowFields(row, volumeAdd, float64(index+1), volumeFields[channel])
		scaleRowFields(row, volumeMul, volumeFields[channel])
	}
}

func scaleRowFields(row map[string]any, factor float64, keys []string) {
	if factor == 1 {
		return
	}

	for _, key := range keys {
		value, ok := row[key].(float64)

		if !ok {
			continue
		}

		row[key] = roundField(value * factor)
	}
}

func addRowFields(row map[string]any, step float64, index float64, keys []string) {
	if step == 0 {
		return
	}

	for _, key := range keys {
		value, ok := row[key].(float64)

		if !ok {
			continue
		}

		row[key] = roundField(math.Max(value+step*index, 0))
	}
}

func marshalFrame(frame map[string]any) []byte {
	payload, err := sonic.Marshal(frame)

	if err != nil {
		panic(err)
	}

	return payload
}
