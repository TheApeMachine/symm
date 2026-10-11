package system

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
)

type Risk struct {
	UncertaintyScale float64
	DrawdownPadding  float64
	MaxFraction      *decimal.Decimal
}

func NewRisk() *Risk {
	viper.SetDefault("risk.uncertainty_scale", 1.0)
	viper.SetDefault("risk.drawdown_padding", 0.005)
	viper.SetDefault("risk.max_fraction", 0.20)

	return &Risk{
		UncertaintyScale: viper.GetFloat64("risk.uncertainty_scale"),
		DrawdownPadding:  viper.GetFloat64("risk.drawdown_padding"),
		MaxFraction:      decimal.NewFromFloat64(viper.GetFloat64("risk.max_fraction")),
	}
}
