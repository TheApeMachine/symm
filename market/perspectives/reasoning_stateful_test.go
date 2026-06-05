package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func notHoldingAnd(inner Predicate) Predicate {
	return Predicate{All: []Predicate{
		{Subject: SubjectPosition, Op: ComparisonEquals, Lifecycle: ObservationNotHolding},
		inner,
	}}
}

// priceRoseBy builds a "price rose by value% over `ago` distinct prices" predicate.
func priceRoseBy(ago int, value float64) Predicate {
	return Predicate{Subject: SubjectPrice, Unit: UnitPercentage, Ago: ago, Op: ComparisonRoseBy, Value: value}
}

func TestEvaluateStatefulArmsAcrossTicks(t *testing.T) {
	Convey("Given an ordered chain — see an ignition, THEN later when price follows through, enter", t, func() {
		tree := []Thought{{
			When: notHoldingAnd(sig(CategoryVerticalIgnition, 1.0)),
			Then: []Thought{{
				When: priceRoseBy(3, 2.0),
				Do:   Act{Type: ActionMarket},
			}},
		}}

		// Tick 1: the ignition is present, but price is still flat.
		tick1 := mockReason{
			lifecycle: map[ObservationType]bool{ObservationNotHolding: true},
			signal:    map[CategoryType][]float64{CategoryVerticalIgnition: {1.5, 1.4, 1.3, 1.2}},
			price:     []float64{100, 100, 100, 100},
		}
		// Tick 2: the ignition is GONE, but price has now followed through (+3% vs 3-ago).
		tick2 := mockReason{
			lifecycle: map[ObservationType]bool{ObservationNotHolding: true},
			signal:    map[CategoryType][]float64{CategoryVerticalIgnition: {0, 0, 0, 0}},
			price:     []float64{103, 102, 101, 100},
		}

		Convey("With persisted state the ignition latches, and the later follow-through fires the entry", func() {
			state := NewReasonState()

			_, found1 := EvaluateStateful(tree, tick1, state)
			So(found1, ShouldBeFalse) // parent fired, child not yet — no action this tick

			act2, found2 := EvaluateStateful(tree, tick2, state)
			So(found2, ShouldBeTrue) // the latched gate lets the follow-through through
			So(act2.Type, ShouldEqual, ActionMarket)
		})

		Convey("Without memory the second tick alone never reaches the entry (the gate is shut)", func() {
			// Single-tick: at tick 2 the ignition is gone, so the parent gate is closed
			// and the child is unreachable. This is exactly what the latch fixes.
			_, found := Evaluate(tree, tick2)
			So(found, ShouldBeFalse)
		})
	})
}

func TestEvaluateStatefulResetsOnHoldingFlip(t *testing.T) {
	Convey("An armed entry frontier is cleared when the position opens (an episode boundary)", t, func() {
		tree := []Thought{{
			When: sig(CategoryVerticalIgnition, 1.0), // a gate that latches, with no action of its own
			Then: []Thought{{
				When: priceRoseBy(2, 2.0),
				Do:   Act{Type: ActionMarket},
			}},
		}}

		// Arms the parent gate: ignition present, price flat so the child holds off.
		armTick := mockReason{
			lifecycle: map[ObservationType]bool{ObservationNotHolding: true},
			signal:    map[CategoryType][]float64{CategoryVerticalIgnition: {1.5, 1.4, 1.3}},
			price:     []float64{100, 100, 100},
		}
		// The follow-through tick: ignition gone, price has now climbed +3% vs 2-ago.
		followTick := func(holding bool) mockReason {
			return mockReason{
				lifecycle: map[ObservationType]bool{ObservationHolding: holding, ObservationNotHolding: !holding},
				signal:    map[CategoryType][]float64{CategoryVerticalIgnition: {0, 0, 0}},
				price:     []float64{103, 101, 100},
			}
		}

		Convey("No holding flip: the latched gate lets the follow-through fire", func() {
			state := NewReasonState()

			EvaluateStateful(tree, armTick, state)
			_, found := EvaluateStateful(tree, followTick(false), state)

			So(found, ShouldBeTrue)
		})

		Convey("Holding flips true between the ticks: the frontier resets and the gate is shut", func() {
			state := NewReasonState()

			EvaluateStateful(tree, armTick, state)
			_, found := EvaluateStateful(tree, followTick(true), state) // holding flip → reset

			So(found, ShouldBeFalse)
		})
	})
}

func TestEvaluateStatefulMatchesSingleTickWhenSimultaneous(t *testing.T) {
	Convey("A tree whose whole chain is true on one tick fires the deepest action, like the single-tick walk", t, func() {
		tree := []Thought{{
			When: notHoldingAnd(sig(CategoryCoiledCompression, 1.0)),
			Then: []Thought{{
				When: sig(CategoryVerticalIgnition, 1.0),
				Do:   Act{Type: ActionIceberg},
			}},
		}}

		ctx := mockReason{
			lifecycle: map[ObservationType]bool{ObservationNotHolding: true},
			signal: map[CategoryType][]float64{
				CategoryCoiledCompression: {1.2},
				CategoryVerticalIgnition:  {1.5},
			},
		}

		stateful, foundStateful := EvaluateStateful(tree, ctx, NewReasonState())
		single, foundSingle := Evaluate(tree, ctx)

		So(foundStateful, ShouldBeTrue)
		So(foundSingle, ShouldBeTrue)
		So(stateful.Type, ShouldEqual, ActionIceberg)
		So(single.Type, ShouldEqual, stateful.Type)
	})
}

func TestReasonStateReset(t *testing.T) {
	Convey("Given a primed ReasonState", t, func() {
		state := NewReasonState()
		state.active["0"] = true
		state.next["1"] = true
		state.lastHolding = true
		state.primed = true

		Convey("Reset clears state without replacing the backing maps", func() {
			active := state.active
			next := state.next

			state.Reset()

			So(len(state.active), ShouldEqual, 0)
			So(len(state.next), ShouldEqual, 0)
			So(state.primed, ShouldBeFalse)
			So(state.lastHolding, ShouldBeFalse)

			state.active["2"] = true
			state.next["3"] = true

			So(active["2"], ShouldBeTrue)
			So(next["3"], ShouldBeTrue)
		})
	})
}

func TestEvaluateStatefulUniversalPositionManager(t *testing.T) {
	Convey("Given the universal position manager branch", t, func() {
		manager := []Thought{{
			When: Predicate{Subject: SubjectPosition, Op: ComparisonEquals, Lifecycle: ObservationHolding},
			Then: []Thought{
				{
					When: Predicate{Subject: SubjectPosition, Op: ComparisonEquals, Lifecycle: ObservationHasStarted},
					Do:   Act{Type: ActionStopLoss, Offset: 0.010},
				},
				{
					When: Predicate{
						Subject: SubjectSignal, Category: CategoryActiveReversal,
						Unit: UnitSNR, Ago: 3, Op: ComparisonCrossedUp, Value: 1.0,
					},
					Do: Act{Type: ActionSettlePosition},
				},
			},
		}}

		Convey("It arms protection on has_started and exits on reversal while holding", func() {
			state := NewReasonState()
			started := mockReason{
				lifecycle: map[ObservationType]bool{
					ObservationHolding:    true,
					ObservationHasStarted: true,
				},
			}

			stop, foundStop := EvaluateStateful(manager, started, state)
			So(foundStop, ShouldBeTrue)
			So(stop.Type, ShouldEqual, ActionStopLoss)

			reversal := mockReason{
				lifecycle: map[ObservationType]bool{ObservationHolding: true},
				signal: map[CategoryType][]float64{
					CategoryActiveReversal: {2.0, 1.0, 0.5, 0.2},
				},
			}

			exit, foundExit := EvaluateStateful(manager, reversal, state)
			So(foundExit, ShouldBeTrue)
			So(exit.Type, ShouldEqual, ActionSettlePosition)
		})
	})
}

func BenchmarkEvaluateStateful(benchmark *testing.B) {
	tree := []Thought{{
		When: notHoldingAnd(sig(CategoryCoiledCompression, 1.0)),
		Then: []Thought{{
			When: sig(CategoryVerticalIgnition, 1.0),
			Do:   Act{Type: ActionIceberg},
		}},
	}}
	context := mockReason{
		lifecycle: map[ObservationType]bool{ObservationNotHolding: true},
		signal: map[CategoryType][]float64{
			CategoryCoiledCompression: {1.2},
			CategoryVerticalIgnition:  {1.5},
		},
	}
	state := NewReasonState()

	for benchmark.Loop() {
		state.Reset()
		_, _ = EvaluateStateful(tree, context, state)
	}
}
