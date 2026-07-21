package instrument

import (
	"embed"
	"fmt"
	"iter"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	return fixture.sequencer(raw)
}

/*
NewMarket injects the requested simulated symbols into the Kraken instrument
snapshot template and returns a ready-to-consume fixture.
*/
func NewMarket(symbols []string, priceIncrement float64) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(SNAPSHOT) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture load failed", err))
	}

	var payload map[string]any

	if err := sonic.Unmarshal(raw, &payload); err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture decode failed", err))
	}

	data := payload["data"].(map[string]any)
	template := data["pairs"].([]any)[0].(map[string]any)
	pairs := make([]map[string]any, len(symbols))

	for index, symbol := range symbols {
		pair := make(map[string]any, len(template))

		for key, value := range template {
			pair[key] = value
		}

		parts := strings.Split(symbol, "/")

		if len(parts) != 2 {
			panic(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("instrument fixture invalid symbol %q", symbol),
				nil,
			))
		}

		pair["symbol"] = symbol
		pair["base"] = parts[0]
		pair["quote"] = parts[1]
		pair["tick_size"] = priceIncrement
		pair["price_increment"] = priceIncrement
		pairs[index] = pair
	}

	data["pairs"] = pairs
	encoded, err := sonic.Marshal(payload)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture encode failed", err))
	}

	return &Fixture{sequence: [][]byte{encoded}}
}

func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "instrument fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, pair := range pairs(step) {
			pair["tick_size"] = pair["tick_size"].(float64) + float64(i)*pair["price_increment"].(float64)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "instrument fixture marshal failed", err))
		}

		fixture.sequence[i] = marshaled
	}

	return fixture
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, seq := range fixture.sequence {
			if !yield(seq) {
				return
			}
		}
	}
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "instrument fixture clone decode failed", err))
	}

	return out
}

func pairs(frame map[string]any) []map[string]any {
	data := frame["data"].(map[string]any)
	out := []map[string]any{}

	for _, item := range data["pairs"].([]any) {
		out = append(out, item.(map[string]any))
	}

	return out
}
