package system

import "github.com/spf13/viper"

type PumpDump struct {
	Capacity int
	Halflife float64
}

func NewPumpDump() *PumpDump {
	viper.SetDefault("pumpdump.capacity", 100)
	viper.SetDefault("pumpdump.ladder_halflife", "30s")

	return &PumpDump{
		Capacity: viper.GetInt("pumpdump.capacity"),
		Halflife: viper.GetDuration("pumpdump.ladder_halflife").Seconds(),
	}
}
