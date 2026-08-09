package system

import "github.com/spf13/viper"

type Risk struct {
	UncertaintyScale float64
	DrawdownPadding  float64
}

func NewRisk() *Risk {
	viper.SetDefault("risk.uncertainty_scale", 1.0)
	viper.SetDefault("risk.drawdown_padding", 0.005)

	return &Risk{
		UncertaintyScale: viper.GetFloat64("risk.uncertainty_scale"),
		DrawdownPadding:  viper.GetFloat64("risk.drawdown_padding"),
	}
}
