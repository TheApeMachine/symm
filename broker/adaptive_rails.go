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
	maxLoss, arm, base, min := 2.0, 0.5, 1.2, 0.45

	if system.Cfg != nil && system.Cfg.Risk != nil {
		risk := system.Cfg.Risk

		if risk.MaxAllowedLossPct > 0 {
			maxLoss = risk.MaxAllowedLossPct
		}

		if risk.TrailPeakArmPct > 0 {
			arm = risk.TrailPeakArmPct
		}

		if risk.TrailBaseGivebackPct > 0 {
			base = risk.TrailBaseGivebackPct
		}

		if risk.TrailMinGivebackPct > 0 {
			min = risk.TrailMinGivebackPct
		}
	}

	rails.maxLoss = maxLoss
	rails.armPct = arm
	rails.baseGiveback = base
	rails.minGiveback = min
}

/*
Snapshot copies the current rails for one lot.
*/
func (rails *AdaptiveRails) Snapshot() RailsSnapshot {
	if rails == nil {
		fallback := NewAdaptiveRails()
		return fallback.Snapshot()
	}

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
	if rails == nil {
		return 0
	}

	rails.mu.Lock()
	defer rails.mu.Unlock()

	if rails.episodes == 0 {
		return 0
	}

	return rails.edgeSum / float64(rails.episodes)
}

func (rails *AdaptiveRails) Episodes() int {
	if rails == nil {
		return 0
	}

	rails.mu.Lock()
	defer rails.mu.Unlock()

	return rails.episodes
}

/*
Learn nudges rails from one finished A/B pair. shadowEarlier is true when the
learning stop would have exited before (or as) the official structural exit.
*/
func (rails *AdaptiveRails) Learn(officialPct, shadowPct float64) {
	if rails == nil {
		return
	}

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
		// Shadow cut was better — tighten giveback / arm earlier next time.
		rails.baseGiveback = clampRail(rails.baseGiveback-step, 0.35, 2.5)
		rails.minGiveback = clampRail(rails.minGiveback-step*0.5, 0.2, 1.5)
		rails.armPct = clampRail(rails.armPct-step*0.25, 0.25, 2.0)
		rails.maxLoss = clampRail(rails.maxLoss-step*0.5, 1.0, 4.0)

	case edge < -0.05:
		// Shadow was worse — either shook out early or held a loser; widen.
		rails.baseGiveback = clampRail(rails.baseGiveback+step, 0.35, 2.5)
		rails.minGiveback = clampRail(rails.minGiveback+step*0.5, 0.2, 1.5)
		rails.armPct = clampRail(rails.armPct+step*0.25, 0.25, 2.0)
		rails.maxLoss = clampRail(rails.maxLoss+step*0.5, 1.0, 4.0)
	}
}

func clampRail(value, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, value))
}
