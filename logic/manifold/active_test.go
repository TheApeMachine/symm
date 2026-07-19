package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

/*
rankedCandidates builds intensity-sorted candidates for selection tests without
touching the GPU engine or an L3 book.
*/
func rankedCandidates() []intensityCandidate {
	return []intensityCandidate{
		{symbol: "AAA/USD", intensity: 10},
		{symbol: "BBB/USD", intensity: 8},
		{symbol: "CCC/USD", intensity: 5},
		{symbol: "DDD/USD", intensity: 3},
	}
}

func TestActiveSetSelectAdvance(t *testing.T) {
	viper.Set("manifold.max_active", 2)
	viper.Set("ui.manifold_focus", "")
	t.Cleanup(func() { viper.Set("manifold.max_active", 0) })

	Convey("Given more booked candidates than the resident budget", t, func() {
		Convey("It admits only the highest-intensity candidates up to budget", func() {
			set := newActiveSet(types.NewThesis(nil, nil))
			selected := set.selectAdvance(rankedCandidates())

			So(selected, ShouldHaveLength, 2)
			So(selected[0].symbol, ShouldEqual, "AAA/USD")
			So(selected[1].symbol, ShouldEqual, "BBB/USD")
		})

		Convey("It always admits protected inventory before filling by intensity", func() {
			thesis := types.NewThesis(nil, nil)
			thesis.Lifecycle.Store("DDD/USD", types.LifecycleManaging)
			set := newActiveSet(thesis)
			selected := set.selectAdvance(rankedCandidates())

			So(selected, ShouldHaveLength, 2)
			So(selected[0].symbol, ShouldEqual, "DDD/USD")
			So(selected[1].symbol, ShouldEqual, "AAA/USD")
		})
	})
}

func TestActiveSetEvict(t *testing.T) {
	viper.Set("manifold.max_active", 2)
	viper.Set("ui.manifold_focus", "AAA/USD")
	t.Cleanup(func() {
		viper.Set("manifold.max_active", 0)
		viper.Set("ui.manifold_focus", "")
	})

	Convey("Given more resident fields than the budget", t, func() {
		now := time.Unix(10_000, 0)
		solver := &Solver{symbols: map[string]*symbolSlot{
			"AAA/USD": {at: now.Add(-time.Hour)},
			"BBB/USD": {at: now.Add(-time.Hour)},
			"CCC/USD": {at: now.Add(-30 * time.Minute)},
			"DDD/USD": {at: now},
		}}
		set := newActiveSet(types.NewThesis(nil, nil))

		Convey("It evicts the coldest non-protected fields down to budget", func() {
			set.evict(solver, now)

			So(len(solver.symbols), ShouldEqual, 2)
			_, protectedResident := solver.symbols["AAA/USD"]
			_, warmResident := solver.symbols["DDD/USD"]
			So(protectedResident, ShouldBeTrue)
			So(warmResident, ShouldBeTrue)
			So(solver.symbols["BBB/USD"], ShouldBeNil)
		})

		Convey("It keeps recently active fields warm despite exceeding budget", func() {
			solver.symbols["BBB/USD"].at = now
			solver.symbols["CCC/USD"].at = now
			set.evict(solver, now)

			So(len(solver.symbols), ShouldEqual, 4)
		})
	})
}
