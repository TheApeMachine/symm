package causal

import (
	"time"

	"github.com/theapemachine/nomagique/correlation"
)

const contagionSymbolCap = 16

func contagionConfig() correlation.ContagionConfig {
	return loadRuntimeConfig().contagionConfig()
}

/*
contagion measures cross-asset coupling across the subscribed universe as the median absolute
Hayashi-Yoshida correlation over symbol pairs. Crypto venues are normally correlated, so it is
the spike toward one — every asset moving as a single block during a liquidation cascade — that
flips the structural causal model into its panic regime.
*/
func (system *System) contagion(at time.Time) float64 {
	if !at.IsZero() && at.Equal(system.contagionAt) {
		return system.contagionCache
	}

	if system.contagionEstimator == nil {
		system.contagionEstimator = correlation.NewContagion(contagionConfig())
	}

	system.contagionCache = system.contagionEstimator.Observe(system.hySnapshots())
	system.contagionAt = at

	return system.contagionCache
}

func (system *System) hySnapshots() []correlation.WindowSnapshot {
	snapshots := make([]correlation.WindowSnapshot, 0)

	system.symbols.Range(func(key, value any) bool {
		state := value.(*CausalSymbol)
		snapshots = append(snapshots, state.HYWindowSnapshot())

		return true
	})

	return snapshots
}
