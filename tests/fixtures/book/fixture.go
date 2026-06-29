package book

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
		panic(errnie.Err(errnie.Validation, "book fixture load failed", err))
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
		panic(errnie.Err(errnie.Validation, "book fixture horizon must be positive", nil))
	}

	var base map[string]any

	if err := sonic.Unmarshal(raw, &base); err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture decode failed", err))
	}

	fixture.sequence = make([][]byte, fixture.horizon)

	for i := range fixture.horizon {
		step := clone(base)

		for _, row := range rows(step) {
			advanceLevels(row, "bids", i, -1)
			advanceLevels(row, "asks", i, 1)
			row["checksum"] = uint64(number(row, "checksum")) + uint64(i+1)
			row["timestamp"] = advance(row["timestamp"].(string), time.Duration(i+1)*250*time.Millisecond)
		}

		marshaled, err := sonic.Marshal(step)

		if err != nil {
			panic(errnie.Err(errnie.Validation, "book fixture marshal failed", err))
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

func advanceLevels(row map[string]any, side string, step int, direction float64) {
	levels := row[side].([]any)

	for i, raw := range levels {
		level := raw.(map[string]any)
		level["price"] = round(number(level, "price") + direction*0.0001*float64(step+1+i))
		level["qty"] = round(math.Max(number(level, "qty")*(1+0.01*float64(step+1)), 0))
	}
}

func advance(value string, delta time.Duration) string {
	observed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture timestamp parse failed", err))
	}

	return observed.Add(delta).Format(time.RFC3339Nano)
}

func round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}

func clone(base map[string]any) map[string]any {
	raw, err := sonic.Marshal(base)

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture clone failed", err))
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture clone decode failed", err))
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
