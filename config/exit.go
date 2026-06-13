package config

/*
ExitConfig is a placeholder for desk hot-reload wiring; trail geometry is derived
at runtime from spread and tape volatility in broker/trail_derive.go.
*/
type ExitConfig struct{}

func LoadExitConfig() (ExitConfig, error) {
	return ExitConfig{}, nil
}

func (config ExitConfig) Float(string, float64) float64 {
	return 0
}
