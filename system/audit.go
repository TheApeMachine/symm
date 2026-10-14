package system

import "github.com/spf13/viper"

type Audit struct {
	Epoch        int64
	Symbol       string
	Ticks        int
	Permutations int
	TakerFee     float64
	OutputDir    string
	NoPlots      bool
	Significance float64
}

func NewAudit() *Audit {
	viper.SetDefault("audit.epoch", 0)
	viper.SetDefault("audit.symbol", "")
	viper.SetDefault("audit.ticks", 1000)
	viper.SetDefault("audit.permutations", 50)
	viper.SetDefault("audit.taker_fee", 0.008)
	viper.SetDefault("audit.output_dir", "audit_results")
	viper.SetDefault("audit.no_plots", false)
	viper.SetDefault("audit.significance", 0.05)

	return &Audit{
		Epoch:        viper.GetInt64("audit.epoch"),
		Symbol:       viper.GetString("audit.symbol"),
		Ticks:        viper.GetInt("audit.ticks"),
		Permutations: viper.GetInt("audit.permutations"),
		TakerFee:     viper.GetFloat64("audit.taker_fee"),
		OutputDir:    viper.GetString("audit.output_dir"),
		NoPlots:      viper.GetBool("audit.no_plots"),
		Significance: viper.GetFloat64("audit.significance"),
	}
}
