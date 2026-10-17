package system

import (
	"errors"
	"fmt"
	"math"

	"github.com/spf13/viper"
)

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
	set          map[string]bool
}

var resonanceKeys = []string{
	"resonance.gate", "resonance.surprise_z", "resonance.learning_rate",
}

func NewResonance() *Resonance {
	viper.SetDefault("resonance.layers", 3)

	resonance := &Resonance{
		Gate:         viper.GetBool("resonance.gate"),
		SurpriseZ:    viper.GetFloat64("resonance.surprise_z"),
		LearningRate: viper.GetFloat64("resonance.learning_rate"),
		Layers:       viper.GetInt("resonance.layers"),
		set:          make(map[string]bool, len(resonanceKeys)),
	}

	for _, key := range resonanceKeys {
		resonance.set[key] = viper.IsSet(key)
	}

	return resonance
}

/*
Validate rejects a gate configuration that is not stated in full or that no
gate can run by: a non-positive pace or a surprise_z that is not finite. The
solver itself refuses a pace above its coders' fastest rate.
*/
func (resonance *Resonance) Validate() error {
	if resonance == nil {
		return fmt.Errorf("resonance configuration is missing")
	}

	var errs []error

	for _, key := range resonanceKeys {
		if !resonance.set[key] {
			errs = append(errs, fmt.Errorf("%s is not configured", key))
		}
	}

	if resonance.LearningRate <= 0 || math.IsNaN(resonance.LearningRate) || math.IsInf(resonance.LearningRate, 0) {
		errs = append(errs, fmt.Errorf("resonance.learning_rate %g is not a positive finite pace", resonance.LearningRate))
	}

	if math.IsNaN(resonance.SurpriseZ) || math.IsInf(resonance.SurpriseZ, 0) {
		errs = append(errs, fmt.Errorf("resonance.surprise_z %g is not finite", resonance.SurpriseZ))
	}

	return errors.Join(errs...)
}
