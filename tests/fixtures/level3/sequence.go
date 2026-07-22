package level3

import (
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
)

/*
sequencer materializes the standalone Level3 fixture used outside the market.
*/
func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "level3 fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for index := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			advanceLevels(row, "bids", index, -1)
			advanceLevels(row, "asks", index, 1)
			row["checksum"] = uint64(number(row, "checksum")) + uint64(index+1)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "level3 fixture marshal failed", err))
		}

		fixture.sequence[index] = marshaled
	}

	return fixture
}

/*
advanceLevels changes standalone template prices, quantities, and timestamps.
*/
func advanceLevels(row map[string]any, side string, step int, direction float64) {
	levels := row[side].([]any)

	for index, raw := range levels {
		level := raw.(map[string]any)
		level["limit_price"] = round(
			number(level, "limit_price") + direction*0.0001*float64(step+1+index),
		)
		level["order_qty"] = round(
			math.Max(number(level, "order_qty")*(1+0.01*float64(step+1)), 0),
		)
		level["timestamp"] = advance(
			level["timestamp"].(string),
			time.Duration(step+1)*250*time.Millisecond,
		)
	}
}

/*
advance moves one exact fixture timestamp by a deterministic interval.
*/
func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

/*
round retains the eight decimal places represented by the wire fixture.
*/
func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

/*
clone isolates one materialized standalone update from its template.
*/
func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "level3 fixture clone decode failed", err))
	}

	return out
}

/*
rows exposes typed Level3 rows from one decoded standalone fixture.
*/
func rows(frame map[string]any) []map[string]any {
	out := []map[string]any{}

	for _, item := range frame["data"].([]any) {
		out = append(out, item.(map[string]any))
	}

	return out
}

/*
number reads one numeric standalone template field.
*/
func number(row map[string]any, key string) float64 {
	return row[key].(float64)
}
