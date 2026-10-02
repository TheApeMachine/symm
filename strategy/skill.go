package strategy

import (
	"fmt"
	"math"
	"sync"
)

/*
Skill tracks empirical trade outcomes (win-rate, edge, sample count)
and determines when the model has demonstrated positive edge to advance
from historical training to forward paper trading.
*/
type Skill struct {
	mu sync.RWMutex

	minSamples int64
	minWinRate float64

	histOpportunities int64
	histCorrectEnter  int64
	histMissedEnter   int64
	histFalseEnter    int64
	histResolved      int64
	histWins          int64
	histEdgeSum       float64
	histEdgeSqSum     float64

	fwdPredictions int64
	fwdPaperTrades int64
	fwdResolved    int64
	fwdWins        int64
	fwdEdgeSum     float64
	fwdEdgeSqSum   float64

	fragmentsUp          int64
	fragmentsDown        int64
	fragmentsChop        int64
	fragmentsFlat        int64
	fragmentsUnsupported int64

	recentSamples []float64
}

func NewSkill() *Skill {
	return &Skill{
		minSamples:    10,
		minWinRate:    0.50,
		recentSamples: make([]float64, 0, 50),
	}
}

func (skill *Skill) Record(pnl float64) {
	skill.RecordHistorical(pnl, pnl > 0)
}

func (skill *Skill) RecordHistorical(pnl float64, correctEnter bool) {
	if skill == nil {
		return
	}

	skill.mu.Lock()
	defer skill.mu.Unlock()

	skill.histOpportunities++
	skill.histResolved++
	skill.histEdgeSum += pnl
	skill.histEdgeSqSum += pnl * pnl

	if pnl > 0 {
		skill.histWins++
	}

	if correctEnter {
		skill.histCorrectEnter++
	} else if pnl > 0 {
		skill.histMissedEnter++
	} else {
		skill.histFalseEnter++
	}

	if len(skill.recentSamples) >= 50 {
		skill.recentSamples = skill.recentSamples[1:]
	}
	skill.recentSamples = append(skill.recentSamples, pnl)
}

func (skill *Skill) RecordForward(pnl float64) {
	if skill == nil {
		return
	}

	skill.mu.Lock()
	defer skill.mu.Unlock()

	skill.fwdPaperTrades++
	skill.fwdResolved++
	skill.fwdEdgeSum += pnl
	skill.fwdEdgeSqSum += pnl * pnl

	if pnl > 0 {
		skill.fwdWins++
	}

	if len(skill.recentSamples) >= 50 {
		skill.recentSamples = skill.recentSamples[1:]
	}
	skill.recentSamples = append(skill.recentSamples, pnl)
}

func (skill *Skill) RecordPrediction() {
	if skill == nil {
		return
	}

	skill.mu.Lock()
	defer skill.mu.Unlock()

	skill.fwdPredictions++
}

func (skill *Skill) RecordFragment(kind string) {
	if skill == nil {
		return
	}

	skill.mu.Lock()
	defer skill.mu.Unlock()

	switch kind {
	case "up":
		skill.fragmentsUp++
	case "down":
		skill.fragmentsDown++
	case "chop":
		skill.fragmentsChop++
	case "flat":
		skill.fragmentsFlat++
	default:
		skill.fragmentsUnsupported++
	}
}

func (skill *Skill) Resolved() int64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	return skill.histResolved + skill.fwdResolved
}

func (skill *Skill) WinRate() float64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	totalResolved := skill.histResolved + skill.fwdResolved
	if totalResolved == 0 {
		return 0
	}

	return float64(skill.histWins+skill.fwdWins) / float64(totalResolved)
}

func (skill *Skill) Edge() float64 {
	if skill == nil {
		return 0
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	totalResolved := skill.histResolved + skill.fwdResolved
	if totalResolved == 0 {
		return 0
	}

	return (skill.histEdgeSum + skill.fwdEdgeSum) / float64(totalResolved)
}

func (skill *Skill) HistOpportunities() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.histOpportunities
}

func (skill *Skill) HistCorrectEnter() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.histCorrectEnter
}

func (skill *Skill) HistMissedEnter() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.histMissedEnter
}

func (skill *Skill) HistFalseEnter() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.histFalseEnter
}

func (skill *Skill) HistMeanReturn() float64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.histResolved == 0 {
		return 0
	}
	return skill.histEdgeSum / float64(skill.histResolved)
}

func (skill *Skill) HistLowerBound() float64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.histResolved < 2 {
		return 0
	}

	n := float64(skill.histResolved)
	mean := skill.histEdgeSum / n
	variance := (skill.histEdgeSqSum - (skill.histEdgeSum*skill.histEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}

	se := math.Sqrt(variance / n)
	return mean - se
}

func (skill *Skill) FwdPredictions() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.fwdPredictions
}

func (skill *Skill) FwdPaperTrades() int64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.fwdPaperTrades
}

func (skill *Skill) FwdMeanReturn() float64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.fwdResolved == 0 {
		return 0
	}
	return skill.fwdEdgeSum / float64(skill.fwdResolved)
}

func (skill *Skill) FwdLowerBound() float64 {
	if skill == nil {
		return 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.fwdResolved < 2 {
		return 0
	}

	n := float64(skill.fwdResolved)
	mean := skill.fwdEdgeSum / n
	variance := (skill.fwdEdgeSqSum - (skill.fwdEdgeSum*skill.fwdEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}

	se := math.Sqrt(variance / n)
	return mean - se
}

func (skill *Skill) Fragments() (up, down, chop, flat, unsupported int64) {
	if skill == nil {
		return 0, 0, 0, 0, 0
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()
	return skill.fragmentsUp, skill.fragmentsDown, skill.fragmentsChop, skill.fragmentsFlat, skill.fragmentsUnsupported
}

func (skill *Skill) RecentSamples() []float64 {
	if skill == nil {
		return nil
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	res := make([]float64, len(skill.recentSamples))
	copy(res, skill.recentSamples)
	return res
}

func (skill *Skill) HistBlocker() string {
	if skill == nil {
		return "skill uninitialized"
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.histResolved == 0 {
		return "accumulating historical training evidence"
	}
	if skill.histResolved < skill.minSamples {
		return fmt.Sprintf("insufficient sample count (have %d, need %d)", skill.histResolved, skill.minSamples)
	}

	n := float64(skill.histResolved)
	mean := skill.histEdgeSum / n
	variance := (skill.histEdgeSqSum - (skill.histEdgeSum*skill.histEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	se := math.Sqrt(variance / n)
	lowerBound := mean - se

	if lowerBound <= 0 {
		return "uncertainty spans zero (need lower_bound > 0)"
	}

	return ""
}

func (skill *Skill) FwdBlocker() string {
	if skill == nil {
		return "skill uninitialized"
	}
	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.fwdResolved < 2 {
		return "no forward paper round trips completed (need >= 2)"
	}

	n := float64(skill.fwdResolved)
	mean := skill.fwdEdgeSum / n
	variance := (skill.fwdEdgeSqSum - (skill.fwdEdgeSum*skill.fwdEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	se := math.Sqrt(variance / n)
	lowerBound := mean - se

	if lowerBound <= 0 {
		return "uncertainty spans zero (need lower_bound > 0)"
	}

	return ""
}

func (skill *Skill) HasEdge() bool {
	if skill == nil {
		return false
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.histResolved < skill.minSamples {
		return false
	}

	winRate := float64(skill.histWins) / float64(skill.histResolved)
	mean := skill.histEdgeSum / float64(skill.histResolved)

	n := float64(skill.histResolved)
	variance := (skill.histEdgeSqSum - (skill.histEdgeSum*skill.histEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	se := math.Sqrt(variance / n)
	lowerBound := mean - se

	return winRate > skill.minWinRate && mean > 0 && lowerBound > 0
}

func (skill *Skill) HasForwardEdge() bool {
	if skill == nil {
		return false
	}

	skill.mu.RLock()
	defer skill.mu.RUnlock()

	if skill.fwdResolved < 2 {
		return false
	}

	n := float64(skill.fwdResolved)
	mean := skill.fwdEdgeSum / n
	variance := (skill.fwdEdgeSqSum - (skill.fwdEdgeSum*skill.fwdEdgeSum)/n) / (n - 1)
	if variance < 0 {
		variance = 0
	}
	se := math.Sqrt(variance / n)
	lowerBound := mean - se

	return mean > 0 && lowerBound > 0
}

