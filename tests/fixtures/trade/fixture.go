package trade

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

/*
FixtureType selects the Kraken trade envelope represented by a fixture.
*/
type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

/*
Fixture yields template-backed standalone or market-driven trade frames.
*/
type Fixture struct {
	horizon  int
	sequence [][]byte
	template []byte
	signal   *marketsignal.Signal
	typ      FixtureType
}

/*
NewFixture loads one trade snapshot or a deterministic update sequence.
*/
func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	return fixture.sequencer(raw)
}

/*
NewMarket creates a trade fixture that renders one Kraken trade per simulated
symbol from each shared market state.
*/
func NewMarket(_ []string, signal *marketsignal.Signal) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture load failed", err))
	}

	return &Fixture{
		template: raw,
		signal:   signal,
		typ:      SNAPSHOT,
	}
}

func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "trade fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			row["price"] = round(number(row, "price") * (1 + 0.001*float64(i+1)))
			row["qty"] = round(math.Max(number(row, "qty")*(1+0.02*float64(i+1)), 0))
			row["trade_id"] = uint64(number(row, "trade_id")) + uint64(i+1)
			row["timestamp"] = advance(row["timestamp"].(string), time.Duration(i+1)*time.Second)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "trade fixture marshal failed", err))
		}

		fixture.sequence[i] = marshaled
	}

	return fixture
}

/*
Generate yields ready Kraken trade payloads in deterministic order.
*/
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

/*
render injects the current state into the Kraken trade template.
*/
func (fixture *Fixture) render(samples []marketsignal.Sample) []byte {
	var payload map[string]any

	if err := sonic.Unmarshal(fixture.template, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture decode failed", err))
	}

	rows := make([]map[string]any, 0, len(samples))

	for _, sample := range samples {
		fills := sample.Fills

		if fixture.typ == SNAPSHOT && len(fills) == 0 && sample.TradeID > 0 {
			fills = []marketsignal.Fill{{
				Side:    sample.Side,
				Price:   sample.TradePrice,
				Qty:     sample.Volume,
				TradeID: sample.TradeID,
				At:      sample.At,
			}}
		}

		for _, fill := range fills {
			row := map[string]any{}

			for key, value := range payload["data"].([]any)[0].(map[string]any) {
				row[key] = value
			}

			row["symbol"] = sample.Symbol
			row["side"] = fill.Side
			row["price"] = fill.Price
			row["qty"] = fill.Qty
			row["trade_id"] = fill.TradeID
			row["timestamp"] = fill.At
			rows = append(rows, row)
		}
	}

	if len(rows) == 0 {
		return nil
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := json.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture encode failed", err))
	}

	fixture.typ = UPDATE

	return encoded
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture clone decode failed", err))
	}

	return out
}

func rows(frame map[string]any) []map[string]any {
	out := []map[string]any{}

	for _, item := range frame["data"].([]any) {
		out = append(out, item.(map[string]any))
	}

	return out
}

func number(row map[string]any, key string) float64 {
	return row[key].(float64)
}
