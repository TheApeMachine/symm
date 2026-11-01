package broker

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/system"
)

/*
AdaptiveRails holds desk-wide stop statics that LearningStop nudges from
official-vs-shadow P/L regret. Snapshots freeze per lot so open episodes
do not chase live updates.
*/
type AdaptiveRails struct {
	mu           sync.Mutex
	maxLoss      float64
	armPct       float64
	baseGiveback float64
	minGiveback  float64
	episodes     int
	edgeSum      float64
}

/*
RailsSnapshot is the frozen rail set one learning episode runs against.
*/
type RailsSnapshot struct {
	MaxLoss      float64
	ArmPct       float64
	BaseGiveback float64
	MinGiveback  float64
}

func NewAdaptiveRails() *AdaptiveRails {
	rails := &AdaptiveRails{}
	rails.resetFromConfig()
	return rails
}

func (rails *AdaptiveRails) resetFromConfig() {
	risk := system.Cfg.Risk
	rails.maxLoss = risk.MaxAllowedLossPct
	rails.armPct = risk.TrailPeakArmPct
	rails.baseGiveback = risk.TrailBaseGivebackPct
	rails.minGiveback = risk.TrailMinGivebackPct
}

/*
Snapshot copies the current rails for one lot.
*/
func (rails *AdaptiveRails) Snapshot() RailsSnapshot {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	return RailsSnapshot{
		MaxLoss:      rails.maxLoss,
		ArmPct:       rails.armPct,
		BaseGiveback: rails.baseGiveback,
		MinGiveback:  rails.minGiveback,
	}
}

/*
Edge is the running mean of (shadowExit − officialExit). Positive means the
learning stop has been beating the active structural exit on average.
*/
func (rails *AdaptiveRails) Edge() float64 {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	if rails.episodes == 0 {
		return 0
	}

	return rails.edgeSum / float64(rails.episodes)
}

func (rails *AdaptiveRails) Episodes() int {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	return rails.episodes
}

/*
Learn nudges rails from one finished A/B pair.
*/
func (rails *AdaptiveRails) Learn(officialPct, shadowPct float64) {
	rails.mu.Lock()
	defer rails.mu.Unlock()

	edge := shadowPct - officialPct
	rails.episodes++
	rails.edgeSum += edge

	step := 0.05 * math.Abs(edge)

	if step < 0.02 {
		step = 0.02
	}

	if step > 0.25 {
		step = 0.25
	}

	switch {
	case edge > 0.05:
		rails.baseGiveback = math.Max(0.35, math.Min(2.5, rails.baseGiveback-step))
		rails.minGiveback = math.Max(0.2, math.Min(1.5, rails.minGiveback-step*0.5))
		rails.armPct = math.Max(0.25, math.Min(2.0, rails.armPct-step*0.25))
		rails.maxLoss = math.Max(1.0, math.Min(4.0, rails.maxLoss-step*0.5))

	case edge < -0.05:
		rails.baseGiveback = math.Max(0.35, math.Min(2.5, rails.baseGiveback+step))
		rails.minGiveback = math.Max(0.2, math.Min(1.5, rails.minGiveback+step*0.5))
		rails.armPct = math.Max(0.25, math.Min(2.0, rails.armPct+step*0.25))
		rails.maxLoss = math.Max(1.0, math.Min(4.0, rails.maxLoss+step*0.5))
	}
}
