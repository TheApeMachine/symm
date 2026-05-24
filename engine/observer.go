package engine

import (
	"iter"

	"github.com/theapemachine/symm/kraken/asset"
)

/*
Observation is a raw data payload from an observer.
*/
type Observation struct {
	Pairs     []asset.Pair
	Timestamp int64
}

/*
Observer is a source of raw data payloads, such as
book, trades, or ticker.
*/
type Observer interface {
	Observe() iter.Seq[Observation]
}
