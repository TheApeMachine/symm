package engine

import (
	"github.com/theapemachine/symm/kraken/asset"
)

type Observation struct {
	Pairs      []asset.Pair
	Confidence float64
	Timestamp  int64
}
