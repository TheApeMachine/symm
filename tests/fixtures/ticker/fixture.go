package ticker

import (
	"embed"
	"iter"

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

/*
Fixture replays an ordered ticker payload sequence for market and broker tests.
The sequence is materialized once so Generate remains deterministic.
*/
type Fixture struct {
	template []byte
	signal   *marketsignal.Signal
	typ      FixtureType
}

/*
NewFixture loads a Kraken ticker payload and expands updates over the requested
horizon while snapshots remain a single exchange frame.
*/
func NewFixture(typ FixtureType, horizon int) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		panic(errnie.Err(errnie.Validation, "ticker fixture load failed", err))
	}

	if typ == SNAPSHOT {
		return &Fixture{template: raw, typ: SNAPSHOT}
	}

	return &Fixture{
		template: raw,
		typ:      UPDATE,
	}
}

/*
Generate yields the materialized Kraken ticker payloads in order.
*/
func (fixture *Fixture) Generate() iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for sample := range fixture.signal.Generate() {
			if !yield(sample) {
				return
			}
		}
	}
}
