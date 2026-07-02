package tests

import (
	"iter"

	"github.com/theapemachine/datura"
)

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

type Fixture interface {
	Generate() iter.Seq[[]byte]
	Artifacts() iter.Seq[*datura.Artifact]
}

func ArtifactSequence(sequence iter.Seq[[]byte]) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		for payload := range sequence {
			artifact := datura.Acquire("fixture", datura.APPJSON).WithPayload(payload)
			role := datura.Peek[string](artifact, "channel")
			scope := datura.Peek[string](artifact, "type")

			if role != "" {
				artifact.WithRole(role)
			}

			if scope != "" {
				artifact.WithScope(scope)
			}

			if !yield(artifact) {
				return
			}
		}
	}
}
