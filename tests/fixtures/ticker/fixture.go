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

type Fixture struct {
	sequence iter.Seq[[]byte]
}

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

	return &Fixture{sequence: engine.Run(raw)}
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return fixture.sequence
}

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
