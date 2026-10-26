package system

import "github.com/spf13/viper"

/*
Resonance configures the chord-level surprise gate (strategy symbolTape):
one resonance solver per command, stepping a predictive coder per symbol at
pace LearningRate over its signals, and a chord the midpoint rule proposes
is appended to the grammar only once the symbol's coder error has settled
(strategy chordGate; no configured warm-up length) and the causal z-score of
its surprise since then reaches SurpriseZ. Gate turns it
off; the settings are still validated and recorded on every policy, so a
gated and an ungated training stay distinguishable. None has a default in
code: the configuration must state each one (cmd/cfg/config.yml).
*/
type Resonance struct {
	Gate         bool
	SurpriseZ    float64
	LearningRate float64
	Layers       int
}

func NewResonance() *Resonance {
	viper.SetDefault("resonance.layers", 3)

	return &Resonance{
		Gate:         viper.GetBool("resonance.gate"),
		SurpriseZ:    viper.GetFloat64("resonance.surprise_z"),
		LearningRate: viper.GetFloat64("resonance.learning_rate"),
		Layers:       viper.GetInt("resonance.layers"),
	}
}
