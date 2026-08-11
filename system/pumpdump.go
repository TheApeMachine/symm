package system

import "github.com/spf13/viper"

type PumpDump struct {
	Capacity int
}

func NewPumpDump() *PumpDump {
	viper.SetDefault("pumpdump.capacity", 100)

	return &PumpDump{
		Capacity: viper.GetInt("pumpdump.capacity"),
	}
}
