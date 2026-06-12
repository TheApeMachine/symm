package causal

import (
	"github.com/theapemachine/nomagique/vector"
)

const (
	l1InputBidPrice = iota
	l1InputAskPrice
	l1InputBidQty
	l1InputAskQty
	l1InputCount
)

const (
	l1FeatureMidPrice = iota
	l1FeatureSpreadBPS
	l1FeatureImbalance
	l1FeatureCount
)

func newL1FeatureExtractor() (*vector.FeatureExtractor, error) {
	return vector.NewFeatureExtractor(l1InputCount,
		func(inputs []float64) float64 {
			return (inputs[l1InputBidPrice] + inputs[l1InputAskPrice]) / 2
		},
		func(inputs []float64) float64 {
			mid := (inputs[l1InputBidPrice] + inputs[l1InputAskPrice]) / 2

			if mid <= 0 {
				return 0
			}

			return (inputs[l1InputAskPrice] - inputs[l1InputBidPrice]) / mid * 10000
		},
		func(inputs []float64) float64 {
			total := inputs[l1InputBidQty] + inputs[l1InputAskQty]

			if total <= 0 {
				return 0
			}

			return (inputs[l1InputBidQty] - inputs[l1InputAskQty]) / total
		},
	)
}
