package optimizer

import (
	"github.com/spf13/viper"
)

/*
Tunables holds numeric search results applied back into viper for the next trial.
*/
type Tunables struct {
	EntryEdgeMultiple      float64
	SpoofWeightedThreshold float64
	MinFillToCancelRatio   float64
	ContagionBreak         float64
	HawkesFitCooldown      float64
	ConditionSwitch        float64
	BookDepthLevels        float64
}

/*
Spec is one searchable dimension with a closed range.
*/
type Spec struct {
	Name string
	Min  float64
	Max  float64
	Step float64
}

/*
DefaultTunables mirrors cmd/cfg/config.yml defaults.
*/
func DefaultTunables() *Tunables {
	return &Tunables{
		EntryEdgeMultiple:      1.0,
		SpoofWeightedThreshold: viper.GetFloat64("signals.spoof_weighted_threshold"),
		MinFillToCancelRatio:   viper.GetFloat64("signals.min_fill_to_cancel_ratio"),
		ContagionBreak:         viper.GetFloat64("signals.causal.contagion_break"),
		HawkesFitCooldown:      viper.GetFloat64("signals.hawkes_fit_cooldown"),
		ConditionSwitch:        float64(viper.GetInt("signals.causal.condition_switch")),
		BookDepthLevels:        float64(viper.GetInt("market.book_depth_levels")),
	}
}

/*
TunableSpecs is the MCTS search space.
*/
func (tunables *Tunables) TunableSpecs() []Spec {
	return []Spec{
		{Name: "entry_edge_multiple", Min: 1.0, Max: 4.0, Step: 0.25},
		{Name: "spoof_weighted_threshold", Min: 0.1, Max: 0.9, Step: 0.05},
		{Name: "min_fill_to_cancel_ratio", Min: 0.05, Max: 0.5, Step: 0.05},
		{Name: "contagion_break", Min: 0.5, Max: 0.99, Step: 0.01},
		{Name: "hawkes_fit_cooldown", Min: 1.0, Max: 15.0, Step: 1.0},
		{Name: "condition_switch", Min: 100.0, Max: 5000.0, Step: 100.0},
		{Name: "book_depth_levels", Min: 5.0, Max: 25.0, Step: 1.0},
	}
}

/*
Apply writes the candidate into viper for the next replay trial.
*/
func (tunables *Tunables) Apply() {
	viper.Set("signals.spoof_weighted_threshold", tunables.SpoofWeightedThreshold)
	viper.Set("signals.min_fill_to_cancel_ratio", tunables.MinFillToCancelRatio)
	viper.Set("signals.causal.contagion_break", tunables.ContagionBreak)
	viper.Set("signals.hawkes_fit_cooldown", tunables.HawkesFitCooldown)
	viper.Set("signals.causal.condition_switch", int(tunables.ConditionSwitch))
	viper.Set("market.book_depth_levels", int(tunables.BookDepthLevels))
}

/*
Clone copies the tunables vector.
*/
func (tunables *Tunables) Clone() *Tunables {
	clone := *tunables

	return &clone
}
