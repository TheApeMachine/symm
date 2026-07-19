package manifold

import (
	"sort"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

/*
defaultMaxActive bounds the resident manifold fields advanced per cut when the
market configuration does not supply manifold.max_active. It keeps per-tick GPU
work proportional to attention rather than to the full quote universe.
*/
const defaultMaxActive = 32

/*
activeSet bounds how many resident manifold fields the solver advances and keeps
warm per cut. Open and pending positions plus the UI focus symbol are always
protected; the remaining budget goes to the highest-intensity candidates. Cold,
non-protected fields are evicted only once the resident count exceeds the budget
and their last advance is older than the retention window, so a brief intensity
dip does not thrash a warm GPU field.
*/
type activeSet struct {
	budget    int
	retention time.Duration
	focus     string
	protected map[string]struct{}
}

/*
newActiveSet reads the resident-field budget from market configuration, derives
the hysteresis retention window from the pair's own baseline timescale, and
captures the open, pending, and focus symbols that must never be evicted.
*/
func newActiveSet(thesis *types.Thesis) activeSet {
	budget := viper.GetInt("manifold.max_active")

	if budget <= 0 {
		budget = defaultMaxActive
	}

	set := activeSet{
		budget:    budget,
		retention: DefaultBaselineHalflife(),
		focus:     viper.GetString("ui.manifold_focus"),
		protected: make(map[string]struct{}),
	}

	if set.focus != "" {
		set.protected[set.focus] = struct{}{}
	}

	set.markProtected(thesis)

	return set
}

/*
markProtected records every symbol whose open or pending inventory requires a
resident field for exit management, mirroring the analyzer interest lifecycle so
manifold retention and analysis priority agree on what is inventory.
*/
func (set activeSet) markProtected(thesis *types.Thesis) {
	if thesis == nil || thesis.Lifecycle == nil {
		return
	}

	thesis.Lifecycle.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		switch phase, _ := value.(string); phase {
		case types.LifecycleEntered, types.LifecycleManaging,
			types.LifecycleExitSelected, types.LifecycleExitSubmitted,
			types.LifecyclePartiallyEntered, types.LifecyclePartiallyExited,
			types.LifecycleEntrySelected, types.LifecycleEntrySubmitted:
			set.protected[symbol] = struct{}{}
		}

		return true
	})
}

/*
isProtected reports whether a symbol is inventory or the UI focus and therefore
exempt from the intensity budget and from eviction.
*/
func (set activeSet) isProtected(symbol string) bool {
	_, ok := set.protected[symbol]

	return ok
}

/*
selectAdvance returns the candidates the solver should step this cut: every
protected symbol first, then the highest-intensity remainder until the budget is
full. Candidates arrive pre-sorted by intensity, so the fill preserves rank and
a genuine high-intensity newcomer is never starved by warm residents.
*/
func (set activeSet) selectAdvance(
	candidates []intensityCandidate,
) []intensityCandidate {
	selected := make([]intensityCandidate, 0, len(candidates))
	chosen := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		if !set.isProtected(candidate.symbol) {
			continue
		}

		selected = append(selected, candidate)
		chosen[candidate.symbol] = struct{}{}
	}

	for _, candidate := range candidates {
		if len(chosen) >= set.budget {
			break
		}

		if _, done := chosen[candidate.symbol]; done {
			continue
		}

		selected = append(selected, candidate)
		chosen[candidate.symbol] = struct{}{}
	}

	return selected
}

/*
evict releases the coldest non-protected resident fields once the resident count
exceeds the budget. A field advanced within the retention window is kept warm so
intensity jitter around the budget boundary does not thrash GPU allocations.
*/
func (set activeSet) evict(solver *Solver, now time.Time) {
	if solver == nil || len(solver.symbols) <= set.budget || now.IsZero() {
		return
	}

	victims := set.coldResidents(solver, now)

	sort.Slice(victims, func(left, right int) bool {
		return victims[left].at.Before(victims[right].at)
	})

	for _, victim := range victims {
		if len(solver.symbols) <= set.budget {
			return
		}

		solver.release(victim.symbol)
	}
}

/*
coldResident is one evictable resident field and the event time of its last
advance, used to release the least recently active fields first.
*/
type coldResident struct {
	symbol string
	at     time.Time
}

/*
coldResidents lists the non-protected resident fields whose last advance is
older than the retention window.
*/
func (set activeSet) coldResidents(solver *Solver, now time.Time) []coldResident {
	cold := make([]coldResident, 0, len(solver.symbols))

	for symbol, slot := range solver.symbols {
		if slot == nil || set.isProtected(symbol) {
			continue
		}

		if now.Sub(slot.at) <= set.retention {
			continue
		}

		cold = append(cold, coldResident{symbol: symbol, at: slot.at})
	}

	return cold
}
