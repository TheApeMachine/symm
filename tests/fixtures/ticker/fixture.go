package ticker

import (
	"embed"
	"encoding/json"
	"iter"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

type Fixture struct {
	horizon  int
	template []byte
	signal   *marketsignal.Signal
	typ      FixtureType
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
	}

	return &Fixture{horizon: horizon}
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	if fixture.signal != nil {
		return func(yield func([]byte) bool) {
			for samples := range fixture.signal.Generate() {
				if !yield(fixture.render(samples)) {
					return
				}
			}
		}
	}

	return func(yield func([]byte) bool) {
		for _, seq := range fixture.sequence {
			if !yield(seq) {
				return
			}
		}
	}
}

func (fixture *Fixture) render(samples []marketsignal.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture decode failed", err))
	}

	rows := make([]map[string]any, 0, len(samples))

	for _, sample := range samples {
		bidPrice := sample.Price
		bidQty := 100.0

		if len(sample.Bids) > 0 {
			bidPrice = sample.Bids[0].Price
			bidQty = sample.Bids[0].Qty
		}

		askPrice := sample.Price
		askQty := 100.0

		if len(sample.Asks) > 0 {
			askPrice = sample.Asks[0].Price
			askQty = sample.Asks[0].Qty
		}

		row := map[string]any{
			"symbol":     sample.Symbol,
			"bid":        bidPrice,
			"bid_qty":    bidQty,
			"ask":        askPrice,
			"ask_qty":    askQty,
			"last":       sample.TradePrice,
			"volume":     sample.Statistics.Volume,
			"vwap":       sample.Statistics.VWAP,
			"low":        sample.Statistics.Low,
			"high":       sample.Statistics.High,
			"change":     sample.TradePrice - sample.Statistics.Open,
			"change_pct": ((sample.TradePrice - sample.Statistics.Open) / sample.Statistics.Open) * 100.0,
			"timestamp":  sample.At.Format(time.RFC3339Nano),
		}
		rows = append(rows, row)
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := json.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture encode failed", err))
	}

	fixture.typ = UPDATE
	return encoded
}

func advanceTime(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func roundVal(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func cloneMap(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture clone decode failed", err))
	}

	return out
}

func rowsMap(frame map[string]any) []map[string]any {
	out := []map[string]any{}

	for _, item := range frame["data"].([]any) {
		out = append(out, item.(map[string]any))
	}

	return out
}

func numberVal(row map[string]any, key string) float64 {
	return row[key].(float64)
}
