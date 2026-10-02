package strategy

import "sync"

/*
Skill tracks empirical trade outcomes (win-rate, edge, sample count)
and determines when the model has demonstrated positive edge to advance
from historical training to forward paper trading.
*/
type Skill struct {
	mu         sync.RWMutex
	resolved   int64
	wins       int64
	edgeSum    float64
	minSamples int64
	minWinRate float64
}

func NewSkill() *Skill {
	return &Skill{
		minSamples: 10,
		minWinRate: 0.50,
	}
}

func (skill *Skill) Record(pnl float64) {
	if skill == nil {
		return
	}

	skill.mu.Lock()
	defer skill.mu.Unlock()

	skill.resolved++
	skill.edgeSum += pnl

	if pnl > 0 {
		skill.wins++
	}
}

func (skill *Skill) Resolved() int64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	return skill.resolved
}

func (skill *Skill) WinRate() float64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.resolved == 0 {
		return 0
	}

	return float64(skill.wins) / float64(skill.resolved)
}

func (skill *Skill) Edge() float64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.resolved == 0 {
		return 0
	}

	return skill.edgeSum / float64(skill.resolved)
}

func (skill *Skill) HasEdge() bool {
	if skill == nil {
		return false
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.resolved < skill.minSamples {
		return false
	}

	winRate := float64(skill.wins) / float64(skill.resolved)
	edge := skill.edgeSum / float64(skill.resolved)

	return winRate > skill.minWinRate && edge > 0
}
