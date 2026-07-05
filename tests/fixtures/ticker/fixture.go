package ticker

import (
	"embed"
	"iter"
	"math"
	"time"

	"github.com/bytedance/sonic"
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
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
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
		panic(errnie.Err(errnie.Validation, "ticker fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			baseLast := number(row, "last")
			last := baseLast * (1 + 0.001*float64(i+1))
			spread := math.Max(number(row, "ask")-number(row, "bid"), baseLast*0.0002)

			row["bid"] = round(last - spread/2)
			row["ask"] = round(last + spread/2)
			row["last"] = round(last)
			row["volume"] = round(number(row, "volume") + 10*float64(i+1))
			row["high"] = round(math.Max(number(row, "high"), last))
			row["low"] = round(math.Min(number(row, "low"), last))
			row["change"] = round(last - baseLast)
			row["change_pct"] = round(((last - baseLast) / baseLast) * 100)
			row["timestamp"] = advance(row["timestamp"].(string), time.Duration(i+1)*5*time.Second)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "ticker fixture marshal failed", err))
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

func (fixture *Fixture) Frames() iter.Seq[tests.Frame] {
	return tests.FrameSequence(fixture.Generate())
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
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
