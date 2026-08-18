package system

import "github.com/spf13/viper"

type PumpDump struct {
	Capacity           int
	Halflife           float64
	FastHalflife       float64
	SlowHalflife       float64
	DispersionHalflife float64
}

func NewPumpDump() *PumpDump {
	viper.SetDefault("pumpdump.capacity", 100)
	viper.SetDefault("pumpdump.ladder_halflife", "30s")
	viper.SetDefault("pumpdump.baseline_fast_halflife", "5s")
	viper.SetDefault("pumpdump.baseline_slow_halflife", "120s")
	viper.SetDefault("pumpdump.dispersion_halflife", "30s")

	return &PumpDump{
		Capacity:           viper.GetInt("pumpdump.capacity"),
		Halflife:           viper.GetDuration("pumpdump.ladder_halflife").Seconds(),
		FastHalflife:       viper.GetDuration("pumpdump.baseline_fast_halflife").Seconds(),
		SlowHalflife:       viper.GetDuration("pumpdump.baseline_slow_halflife").Seconds(),
		DispersionHalflife: viper.GetDuration("pumpdump.dispersion_halflife").Seconds(),
	}
}
