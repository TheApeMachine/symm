package tests

import (
	"embed"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
)

//go:embed fixtures/*.json
var fixtureFiles embed.FS

type FixtureType string

const (
	FixtureTypeTicker     FixtureType = "ticker"
	FixtureTypeBook       FixtureType = "book"
	FixtureTypeCandles    FixtureType = "candles"
	FixtureTypeTrade      FixtureType = "trade"
	FixtureTypeInstrument FixtureType = "instrument"
	FixtureTypeStatus     FixtureType = "status"
	FixtureTypeHeartbeat  FixtureType = "heartbeat"
)

type Fixture struct {
	Type FixtureType
	Data []byte
}

func NewFixture(typ FixtureType) *Fixture {
	raw, err := fixtureFiles.ReadFile("fixtures/" + string(typ) + ".json")

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"fixture load failed",
			err,
		))

		return &Fixture{Type: typ}
	}

	return NewFixtureFromPayload(typ, raw)
}

func NewFixtureFromPayload(typ FixtureType, payload []byte) *Fixture {
	return &Fixture{
		Type: typ,
		Data: payload,
	}
}

func ArtifactFromPayload(payload []byte) *datura.Artifact {
	return NewFixtureFromPayload("", payload).ToArtifact()
}

func ArtifactSequence(sequence iter.Seq[[]byte]) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		for payload := range sequence {
			if !yield(ArtifactFromPayload(payload)) {
				return
			}
		}
	}
}

/*
ToArtifact mirrors kraken/public/websocket.go ingest: raw Kraken JSON on the
payload, channel as role, type as scope.
*/
func (fixture *Fixture) ToArtifact() *datura.Artifact {
	if len(fixture.Data) == 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"fixture payload is empty",
			nil,
		))

		return nil
	}

	artifact := datura.Acquire(
		"kraken:public", datura.APPJSON,
	).WithPayload(fixture.Data)

	if artifact == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"fixture artifact payload rejected",
			nil,
		))

		return nil
	}

	artifact.WithRole(
		datura.Peek[string](artifact, "channel"),
	)

	scope := datura.Peek[string](artifact, "type")

	if scope != "" {
		artifact.WithScope(scope)
	}

	return artifact
}

/*
Ingest writes one Kraken frame into the tree the same way the public websocket
does after WithPayload, WithRole, and WithScope.
*/
func (fixture *Fixture) Ingest(tree *dmt.Tree, timestamp int64) {
	artifact := fixture.ToArtifact()

	if artifact == nil {
		return
	}

	defer artifact.Release()

	if timestamp > 0 {
		artifact.SetTimestamp(timestamp)
	}

	wire := artifact.Pack()

	if len(wire) == 0 {
		return
	}

	tree.Insert(artifact.Prefix("timestamp"), wire)
}

func (fixture *Fixture) InsertIntoTree(tree *dmt.Tree, timestamp int64) {
	fixture.Ingest(tree, timestamp)
}

func (fixture *Fixture) InsertReplay(tree *dmt.Tree, tickCount int, timestamp *int64) {
	for range tickCount {
		*timestamp++
		fixture.Ingest(tree, *timestamp)
	}
}
