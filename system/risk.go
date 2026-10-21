package system

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
)

type Risk struct {
	MaxFraction *decimal.Decimal
}

func NewRisk() *Risk {
	viper.SetDefault("risk.max_fraction", 0.20)

	return &Risk{
		MaxFraction: decimal.NewFromFloat64(viper.GetFloat64("risk.max_fraction")),
	}
}
