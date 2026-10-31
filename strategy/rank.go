package strategy

import (
	"slices"
	"strings"
	"sync"

	"github.com/theapemachine/nomagique/data"
	nmstore "github.com/theapemachine/nomagique/store"
)

/*
Opportunity is one entry candidate scored for this tick. Rank holds them
until the tick's Evaluate pass finishes, then commits best-first.
*/
type Opportunity struct {
	Symbol    string
	Path      string
	Virtual   bool
	PathFound bool
	Score     float64
}

/*
Rank is the per-tick priority queue of entry opportunities.
*/
type Rank struct {
	mu   sync.Mutex
	list []Opportunity
}

func NewRank() *Rank {
	return &Rank{}
}

func (rank *Rank) Offer(opp Opportunity) {
	if rank == nil || opp.Symbol == "" {
		return
	}

	rank.mu.Lock()
	defer rank.mu.Unlock()

	rank.list = append(rank.list, opp)
}

/*
TakeSorted drains the queue highest score first (symbol ascending on ties).
*/
func (rank *Rank) TakeSorted() []Opportunity {
	if rank == nil {
		return nil
	}

	rank.mu.Lock()
	defer rank.mu.Unlock()

	out := rank.list
	rank.list = nil

	if len(out) == 0 {
		return nil
	}

	slices.SortFunc(out, func(a, b Opportunity) int {
		if a.Score > b.Score {
			return -1
		}

		if a.Score < b.Score {
			return 1
		}

		return strings.Compare(a.Symbol, b.Symbol)
	})

	return out
}

/*
Score is (volume acceleration × token brightness) / spread.
Non-positive spread or brightness yields 0. Flat or decelerating
clocks (accel <= 0) yield 0 so grinders never compete for cash.
*/
func Score(accel, brightness, spreadPct float64) float64 {
	if spreadPct <= 0 || brightness <= 0 || accel <= 0 {
		return 0
	}

	return (accel * brightness) / spreadPct
}

func peakBrightness(observed []*nmstore.Region) float64 {
	peak := 0.0

	for _, region := range observed {
		if region == nil {
			continue
		}

		if region.Brightness > peak {
			peak = region.Brightness
		}
	}

	return peak
}

func clockAcceleration(tick *data.Value, symbol string) float64 {
	if tick == nil || symbol == "" {
		return 0
	}

	var vals []float64
	tick.Lookup(&vals, strings.Join([]string{"pumpdump", symbol, "clock_acceleration"}, "/"))

	if len(vals) == 0 {
		return 0
	}

	return vals[0]
}
