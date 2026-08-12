package system

import "github.com/spf13/viper"

/*
RegulatorConfig defines the observable history and posterior confidence used by
the online system optimizer.
*/
type RegulatorConfig struct {
	HistoryCapacity        int
	OptimizationConfidence float64
}

/*
NewRegulatorConfig loads the configured regulator policy.
*/
func NewRegulatorConfig() *RegulatorConfig {
	viper.SetDefault("regulator.history_capacity", 256)
	viper.SetDefault("regulator.optimization_confidence", 0.95)

	return &RegulatorConfig{
		HistoryCapacity:        viper.GetInt("regulator.history_capacity"),
		OptimizationConfidence: viper.GetFloat64("regulator.optimization_confidence"),
	}
}
