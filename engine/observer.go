package engine

import (
	"context"

	"github.com/theapemachine/symm/kraken/asset"
)

type Observation struct {
	Pairs      []asset.Pair
	Confidence float64
	Timestamp  int64
}

/*
Observer marks a Kraken websocket data source.
Implementations hold per-symbol state updated from WS frames; callers
type-assert to the concrete book, trades, or ticker type to read it.
*/
type Observer interface {
	Observe(context.Context) (Observation, error)
}
