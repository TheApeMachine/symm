package book

import (
	"embed"
	"iter"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

type FixtureType string

const (
	SNAPSHOT FixtureType = "snapshot"
	UPDATE   FixtureType = "update"
)

type Fixture struct {
	horizon   int
	template  []byte
	generator *signal.Generator
	typ       FixtureType
}

func NewFixture(
	typ FixtureType,
	horizon int,
	generator *signal.Generator,
) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "book fixture load failed", err))
	}

	return &Fixture{
		horizon:   horizon,
		template:  raw,
		generator: generator,
		typ:       typ,
	}
}

func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for sample := range fixture.generator.Generate(fixture.template) {
			if !yield(sample) {
				return
			}
		}
	}
}

func (fixture *Fixture) Render(sample testtypes.Sample) []byte {
	return fixture.generator.Render(fixture.template, sample)
}
