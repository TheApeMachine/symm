package settings

import "github.com/spf13/viper"

const defaultL3Depth = 10

/*
L3Enabled reports whether a live authenticated Level 3 feed should run alongside
paper execution.
*/
func L3Enabled() bool {
	return viper.GetBool("market.l3_enabled")
}

/*
L3Depth returns the Kraken level3 book depth subscription (10, 100, or 1000).
*/
func L3Depth() int {
	depth := viper.GetInt("market.l3_depth")

	if depth <= 0 {
		return defaultL3Depth
	}

	return depth
}
