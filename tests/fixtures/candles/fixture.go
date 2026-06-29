package candles

import (
	"embed"
	"iter"
	"math"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests"
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
		panic(errnie.Err(errnie.Validation, "candles fixture load failed", err))
	}

	fixture := &Fixture{horizon: horizon}

	if typ == SNAPSHOT {
		fixture.sequence = [][]byte{raw}
		return fixture
	}

	return fixture.sequencer(raw)
}

func (fixture *Fixture) sequencer(raw []byte) *Fixture {
	if fixture.horizon < 1 {
		panic(errnie.Err(errnie.Validation, "candles fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "candles fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)
		step["timestamp"] = advance(step["timestamp"].(string), time.Duration(i+1)*time.Minute)

		for _, row := range rows(step) {
			cadence := time.Duration(number(row, "interval")) * time.Minute
			closeValue := number(row, "close") * (1 + 0.0015*float64(i+1))

			row["close"] = round(closeValue)
			row["high"] = round(math.Max(number(row, "high"), closeValue))
			row["low"] = round(math.Min(number(row, "low"), closeValue))
			row["trades"] = uint64(number(row, "trades")) + uint64(i+1)
			row["volume"] = round(number(row, "volume") + 25*float64(i+1))
			row["vwap"] = round((number(row, "vwap") + closeValue) / 2)
			row["interval_begin"] = advance(row["interval_begin"].(string), time.Duration(i+1)*cadence)
			row["timestamp"] = advance(row["timestamp"].(string), time.Duration(i+1)*cadence)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "candles fixture marshal failed", err))
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

func (fixture *Fixture) Artifacts() iter.Seq[*datura.Artifact] {
	return tests.ArtifactSequence(fixture.Generate())
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "candles fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "candles fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "candles fixture clone decode failed", err))
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
