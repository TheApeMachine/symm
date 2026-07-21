package trade

import (
	"embed"
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
	sequence [][]byte
	template []byte
	signal   *marketsignal.Signal
	previous map[string]float64
	tradeID  uint64
	typ      FixtureType
}

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
func NewMarket(symbols []string, signal *marketsignal.Signal) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "trade fixture decode failed", err))
	}

	row := payload["data"].([]any)[0].(map[string]any)

	return &Fixture{
		template: raw,
		signal:   signal,
		previous: make(map[string]float64, len(symbols)),
		tradeID:  uint64(row["trade_id"].(float64)),
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

	rows := make([]map[string]any, len(samples))

	for index, sample := range samples {
		row := map[string]any{}

		for key, value := range payload["data"].([]any)[0].(map[string]any) {
			row[key] = value
		}

		side := "buy"

		if previous := fixture.previous[sample.Symbol]; previous > sample.Price {
			side = "sell"
		}

		fixture.tradeID++
		row["symbol"] = sample.Symbol
		row["side"] = side
		row["price"] = sample.Price
		row["qty"] = sample.Volume
		row["trade_id"] = fixture.tradeID
		row["timestamp"] = sample.At
		rows[index] = row
		fixture.previous[sample.Symbol] = sample.Price
	}

	payload["data"] = rows
	payload["type"] = string(fixture.typ)
	encoded, err := sonic.Marshal(payload)

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
