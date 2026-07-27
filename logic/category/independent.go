package category

import (
	"math"

	"github.com/theapemachine/symm/types"
)

var independentMetrics = [...]types.MetricType{
	types.MetricDecoupled,
	types.MetricNoiseScore,
}

/*
pairMemory accumulates per-symbol category activation and co-activation mass so
IndependenceOf can be tested against the product baseline P(A)P(B) without a
fixed window: expected joint mass is (massA/total)*(massB/total)*total.
*/
type pairMemory struct {
	solo  map[nodeKey]float64
	joint map[pairKey]float64
	total map[string]float64
}

type pairKey struct {
	symbol string
	left   uint8
	right  uint8
}

func makePairKey(symbol string, left, right types.CategoryType) pairKey {
	lID := categoryID(left)
	rID := categoryID(right)

	if rID < lID {
		lID, rID = rID, lID
	}

	return pairKey{
		symbol: symbol,
		left:   lID,
		right:  rID,
	}
}

/*
newPairMemory allocates empty activation memories.
*/
func newPairMemory() *pairMemory {
	return &pairMemory{
		solo:  map[nodeKey]float64{},
		joint: map[pairKey]float64{},
		total: map[string]float64{},
	}
}

/*
observe adds solo activation mass for one category on a cut.
*/
func (memory *pairMemory) observe(
	symbol string, categoryType types.CategoryType, strength float64,
) {
	if memory == nil || strength <= 0 {
		return
	}

	memory.solo[makeNodeKey(symbol, categoryType)] += strength
	memory.total[symbol] += strength
}

/*
coobserve adds joint activation mass for an unordered category pair.
*/
func (memory *pairMemory) coobserve(
	symbol string,
	left, right types.CategoryType,
	leftStrength, rightStrength float64,
) {
	if memory == nil || leftStrength <= 0 || rightStrength <= 0 || left == right {
		return
	}

	if right < left {
		leftStrength, rightStrength = rightStrength, leftStrength
	}

	memory.joint[makePairKey(symbol, left, right)] +=
		math.Sqrt(leftStrength * rightStrength)
}

/*
independent reports whether observed joint mass matches the product baseline
more closely than a coupled excess. Returns the independence mass to strengthen
when the pair is independent under that test.
*/
func (memory *pairMemory) independent(
	symbol string,
	left, right types.CategoryType,
	leftStrength, rightStrength float64,
) (mass float64, ok bool) {
	if memory == nil {
		return 0, false
	}

	total := memory.total[symbol]

	if total <= 0 {
		return 0, false
	}

	soloLeft := memory.solo[makeNodeKey(symbol, left)]
	soloRight := memory.solo[makeNodeKey(symbol, right)]
	observed := memory.joint[makePairKey(symbol, left, right)]

	if soloLeft <= 0 || soloRight <= 0 || observed <= 0 {
		return 0, false
	}

	expected := soloLeft * soloRight / total
	coupling := observed - expected

	if coupling >= 0 {
		return 0, false
	}

	// Independence mass is how far joint sits below the product baseline,
	// scaled by the current cut's geometric strength.
	deficit := -coupling / (observed + expected)
	mass = deficit * math.Sqrt(leftStrength*rightStrength)

	return mass, mass > 0
}
