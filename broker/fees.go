package broker

import (
	"github.com/spf13/viper"
)

const (
	defaultTakerFeePctPercent = 0.40
	defaultMakerFeePctPercent = 0.25
)

/*
TakerFeePctFromViper loads the configured taker fee percent per side.

Supported keys:
  - trading.paper.taker_fee_pct
  - trading.paper.fee_pct (alias)
*/
func TakerFeePctFromViper() float64 {
	config := viper.GetViper()

	takerPct := config.GetFloat64("trading.paper.taker_fee_pct")

	if takerPct <= 0 {
		takerPct = config.GetFloat64("trading.paper.fee_pct")
	}

	if takerPct <= 0 {
		takerPct = defaultTakerFeePctPercent
	}

	return takerPct
}

/*
MakerFeePctFromViper loads the configured maker fee percent per side.
*/
func MakerFeePctFromViper() float64 {
	makerPct := viper.GetViper().GetFloat64("trading.paper.maker_fee_pct")

	if makerPct <= 0 {
		makerPct = defaultMakerFeePctPercent
	}

	return makerPct
}
