package broker

import "sync"

/*
LearnedStops holds stop-engine scalars the strategy learns from virtual round
trips. Defaults match the historical fixed constants until gate.bin seeds them.
*/
type learnedStops struct {
	mu              sync.RWMutex
	damageRatio     float64
	evaporationMult float64
}

var LearnedStops learnedStops

func (l *learnedStops) DamageRatio() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.damageRatio <= 0 {
		return 0.75
	}
	return l.damageRatio
}

func (l *learnedStops) EvaporationMult() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.evaporationMult <= 0 {
		return 2.0
	}
	return l.evaporationMult
}

func (l *learnedStops) Set(damageRatio, evaporationMult float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if damageRatio > 0 {
		l.damageRatio = damageRatio
	}
	if evaporationMult > 0 {
		l.evaporationMult = evaporationMult
	}
}
