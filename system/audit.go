package system

import "github.com/spf13/viper"

/*
Audit configures the empirical audit (cmd audit). Every threshold a stage
judges by is declared here (and in cmd/cfg/config.yml), none inside a stage.
TakerFee is a fraction per side; the audit command sets it from its required
--taker-fee-percent flag.
*/
type Audit struct {
	Epoch        int64
	Symbol       string
	Ticks        int
	Permutations int
	TakerFee     float64
	OutputDir    string
	NoPlots      bool
	Significance float64

	MinTokens                int
	MinPrecursorTokens       int
	DominanceShare           float64
	EntropyTolerance         float64
	StationarityJSD          float64
	GridCompleteness         float64
	StagnantZeroFraction     float64
	IdentityTolerance        float64
	LatencyDispersionFloorMs float64
	LatencySpikeFloorMs      float64
	CollinearCorrelation     float64
	ComovementConcordance    float64
}

func NewAudit() *Audit {
	viper.SetDefault("audit.epoch", 0)
	viper.SetDefault("audit.symbol", "")
	viper.SetDefault("audit.ticks", 1000)
	viper.SetDefault("audit.permutations", 50)
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

		MinTokens:                viper.GetInt("audit.min_tokens"),
		MinPrecursorTokens:       viper.GetInt("audit.min_precursor_tokens"),
		DominanceShare:           viper.GetFloat64("audit.dominance_share"),
		EntropyTolerance:         viper.GetFloat64("audit.entropy_tolerance"),
		StationarityJSD:          viper.GetFloat64("audit.stationarity_jsd"),
		GridCompleteness:         viper.GetFloat64("audit.grid_completeness"),
		StagnantZeroFraction:     viper.GetFloat64("audit.stagnant_zero_fraction"),
		IdentityTolerance:        viper.GetFloat64("audit.identity_tolerance"),
		LatencyDispersionFloorMs: viper.GetFloat64("audit.latency_dispersion_floor_ms"),
		LatencySpikeFloorMs:      viper.GetFloat64("audit.latency_spike_floor_ms"),
		CollinearCorrelation:     viper.GetFloat64("audit.collinear_correlation"),
		ComovementConcordance:    viper.GetFloat64("audit.comovement_concordance"),
	}
}
