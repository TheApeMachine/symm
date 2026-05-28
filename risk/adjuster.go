package risk

import (
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/wallet"
)

type Adjuster struct {
	Dampener            float64
	Reason              string
	DrawdownPct         float64
	SystemicCorrelation float64
	HasSystemicMeasure  bool
}

func NewAdjuster() *Adjuster {
	return &Adjuster{Dampener: 1}
}

func (adjuster *Adjuster) Adjust(
	wallet *wallet.Wallet,
	measurement engine.Measurement,
	openSymbols []string,
) Adjuster {
	return adjuster.Adjust(wallet, measurement, openSymbols)
}
