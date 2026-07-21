package heartbeat

import (
	"embed"
	"iter"

	"github.com/theapemachine/errnie"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

type FixtureType string

const UPDATE FixtureType = "update"

type Fixture struct {
	horizon  int
	sequence [][]byte
}

func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "heartbeat fixture load failed", err))
	}

	fixture := &Fixture{
		horizon:  horizon,
		sequence: make([][]byte, horizon),
	}

	if horizon < 1 {
		panic(errnie.Err(errnie.Validation, "heartbeat fixture horizon must be positive", nil))
	}

	for i := range horizon {
		fixture.sequence[i] = raw
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
