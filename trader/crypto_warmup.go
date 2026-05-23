package trader

import "github.com/theapemachine/symm/config"

func (crypto *Crypto) readyForTrading() bool {
	if crypto.engineStats == nil {
		return false
	}

	total := crypto.engineStats.SymbolTotal()

	if total <= 0 {
		return false
	}

	ready := crypto.engineStats.TickerReadyCount()
	coverage := float64(ready) / float64(total)

	if coverage < config.System.MinQuoteCoverage {
		return false
	}

	if crypto.pulseSeq.Load() < int64(config.System.MinWarmPulses) {
		return false
	}

	return true
}
