package ticker

import (
	"embed"
	"iter"
	"time"

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

/*
Fixture replays an ordered ticker payload sequence for market and broker tests.
Payloads are materialized up front so Generate and Frames remain repeatable.
*/
type Fixture struct {
	sequence [][]byte
}

/*
NewFixture loads an embedded ticker template and expands it over the horizon.
*/
func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
	}

	return NewWithEngine(typ, defaultEngine(typ, horizon), raw)
}

/*
NewWithEngine builds a fixture from an explicit sequencer engine.
*/
func NewWithEngine(
	typ FixtureType,
	engine *tests.Engine,
	raw []byte,
) *Fixture {
	if typ == SNAPSHOT {
		engine = tests.NewEngine(1)
	}

	sequence := make([][]byte, 0)

	for payload := range engine.Run(raw) {
		sequence = append(sequence, append([]byte(nil), payload...))
	}

	return &Fixture{sequence: sequence}
}

/*
Generate yields the materialized ticker payloads in order.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for _, payload := range fixture.sequence {
			if !yield(payload) {
				return
			}
		}
	}
}

/*
Frames yields typed channel frames derived from Generate.
*/
func (fixture *Fixture) Frames() iter.Seq[tests.Frame] {
	return tests.FrameSequence(fixture.Generate())
}

func defaultEngine(typ FixtureType, horizon int) *tests.Engine {
	if typ == SNAPSHOT {
		return tests.NewEngine(1)
	}

	return tests.NewEngine(horizon).
		Drift(0.001).
		VolumeAdd(10).
		Interval(5 * time.Second)
}
