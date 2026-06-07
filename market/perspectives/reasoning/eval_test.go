package reasoning

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// mockReason answers predicate queries from ago-indexed series (index 0 = now).
type mockReason struct {
	now       time.Time
	regime    types.Regime
	lifecycle map[types.ObservationType]bool
	signal    map[types.CategoryType][]float64
	price     []float64
	volume    []float64
	elapsed   float64
}

func (m mockReason) Regime() types.Regime                   { return m.regime }
func (m mockReason) PositionSide() trading.Side             { return "" }
func (m mockReason) PositionStrategy() string              { return "" }
func (m mockReason) Now() time.Time                         { return m.now }
func (m mockReason) Lifecycle(s types.ObservationType) bool { return m.lifecycle[s] }

func (m mockReason) Signal(c types.CategoryType, _ UnitType, lookback Lookback) (float64, bool) {
	series, ok := m.signal[c]
	if !ok || lookback.Within > 0 || lookback.Ago < 0 || lookback.Ago >= len(series) {
		return 0, false
	}
	return series[lookback.Ago], true
}

func (m mockReason) Scalar(subject Subject, _ UnitType, lookback Lookback) (float64, bool) {
	var series []float64
	switch subject {
	case SubjectPrice:
		series = m.price
	case SubjectVolume:
		series = m.volume
	case SubjectElapsed:
		return m.elapsed, true
	default:
		return 0, false
	}
	if lookback.Within > 0 || lookback.Ago < 0 || lookback.Ago >= len(series) {
		return 0, false
	}
	return series[lookback.Ago], true
}

func sig(cat types.CategoryType, value float64) Predicate {
	return Predicate{Subject: SubjectSignal, Category: cat, Unit: UnitSNR, Op: ComparisonAtLeast, Value: value}
}

func TestReasoningPredicates(t *testing.T) {
	Convey("Given a reason context", t, func() {
		ctx := mockReason{
			regime:    types.RegimeTrending,
			lifecycle: map[types.ObservationType]bool{types.ObservationHolding: true, types.ObservationHasContinued: true},
			signal: map[types.CategoryType][]float64{
				types.CategoryCoiledCompression: {1.2, 1.1, 1.0, 0.9, 0.8, 0.7}, // now..5-ago
				types.CategoryVerticalIgnition:  {2.0, 1.0, 0.4, 0.2, 0.1, 0.0}, // crossed up recently
				types.CategoryToxicBluff:        {0.0, 0.0},
			},
			price:   []float64{103.0, 102, 101, 101, 100.5, 100.0}, // +3% vs 5-ago
			volume:  []float64{130.0, 120, 110, 105, 102, 100.0},   // +30% vs 5-ago
			elapsed: 35,
		}

		Convey("A level comparison reads the subject now", func() {
			So(holds(sig(types.CategoryCoiledCompression, 1.0), ctx), ShouldBeTrue)
			So(holds(sig(types.CategoryCoiledCompression, 1.5), ctx), ShouldBeFalse)
		})

		Convey("all / any / not compose", func() {
			So(holds(Predicate{All: []Predicate{
				sig(types.CategoryCoiledCompression, 1.0),
				{Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeTrending},
			}}, ctx), ShouldBeTrue)

			So(holds(Predicate{Any: []Predicate{
				sig(types.CategoryCoiledCompression, 5.0), // fails
				sig(types.CategoryVerticalIgnition, 1.5),  // holds
			}}, ctx), ShouldBeTrue)

			So(holds(Predicate{Not: &Predicate{
				Subject: SubjectSignal, Category: types.CategoryToxicBluff, Unit: UnitSNR,
				Op: ComparisonAbove, Value: 0.0,
			}}, ctx), ShouldBeTrue) // toxic is 0, so "above 0" is false, so NOT is true
		})

		Convey("rose_by reads now vs ago as a percentage change", func() {
			So(holds(Predicate{
				Subject: SubjectPrice, Unit: UnitPercentage, Ago: 5, Op: ComparisonRoseBy, Value: 2.0,
			}, ctx), ShouldBeTrue) // +3% >= 2%
			So(holds(Predicate{
				Subject: SubjectPrice, Unit: UnitPercentage, Ago: 5, Op: ComparisonRoseBy, Value: 5.0,
			}, ctx), ShouldBeFalse)
		})

		Convey("crossed_up is edge-triggered over the lookback", func() {
			// ignition was 0.0 at 5-ago and is 2.0 now → crossed above 1.5
			So(holds(Predicate{
				Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: UnitSNR,
				Ago: 5, Op: ComparisonCrossedUp, Value: 1.5,
			}, ctx), ShouldBeTrue)
			// it did not cross 3.0
			So(holds(Predicate{
				Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: UnitSNR,
				Ago: 5, Op: ComparisonCrossedUp, Value: 3.0,
			}, ctx), ShouldBeFalse)
		})

		Convey("metric-to-metric (versus) compares two live subjects", func() {
			// ignition now (2.0) > compression now (1.2)
			So(holds(Predicate{
				Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: UnitSNR, Op: ComparisonAbove,
				Versus: &Operand{Subject: SubjectSignal, Category: types.CategoryCoiledCompression, Unit: UnitSNR},
			}, ctx), ShouldBeTrue)
		})

		Convey("regime and lifecycle are state matches", func() {
			So(holds(Predicate{Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeTrending}, ctx), ShouldBeTrue)
			So(holds(Predicate{Subject: SubjectRegime, Op: ComparisonEquals, Regime: types.RegimeBullish}, ctx), ShouldBeFalse)
			So(holds(Predicate{Subject: SubjectPosition, Op: ComparisonEquals, Lifecycle: types.ObservationHasContinued}, ctx), ShouldBeTrue)
			So(holds(Predicate{Subject: SubjectPosition, Op: ComparisonEquals, Lifecycle: types.ObservationHasEnded}, ctx), ShouldBeFalse)
		})

		Convey("Evaluate returns the deepest reachable decision", func() {
			thoughts := []Thought{
				{
					When: sig(types.CategoryCoiledCompression, 1.0),
					Then: []Thought{
						{
							When: Predicate{Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: UnitSNR, Ago: 5, Op: ComparisonCrossedUp, Value: 1.5},
							Do:   Act{Type: ActionIceberg},
						},
					},
				},
			}

			act, found := Evaluate(thoughts, ctx)

			So(found, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionIceberg)
		})
	})
}

func TestPositionStrategyGate(t *testing.T) {
	Convey("Given a long position opened by quick_pump", t, func() {
		var regime types.Regime

		held := NewWindowReason(nil, regime, PositionState{
			Holding: true, Side: trading.Buy, Strategy: "quick_pump", Last: 100,
		})

		Convey("A matching strategy leaf holds (match implies holding)", func() {
			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "quick_pump", Op: ComparisonEquals,
			}, held), ShouldBeTrue)
		})

		Convey("A different setup's leaf does not hold", func() {
			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "slow_pump", Op: ComparisonEquals,
			}, held), ShouldBeFalse)
		})

		Convey("Strategy conjoins with lifecycle and side", func() {
			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "quick_pump",
				Lifecycle: types.ObservationHolding, Op: ComparisonEquals,
			}, held), ShouldBeTrue)
			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "quick_pump",
				Side: trading.Sell, Op: ComparisonEquals,
			}, held), ShouldBeFalse)
		})

		Convey("A flat symbol never matches any strategy", func() {
			flat := NewWindowReason(nil, regime, PositionState{Holding: false})

			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "quick_pump", Op: ComparisonEquals,
			}, flat), ShouldBeFalse)
		})

		Convey("An unattributed position (legacy dust) matches no strategy leaf", func() {
			legacy := NewWindowReason(nil, regime, PositionState{
				Holding: true, Side: trading.Buy, Last: 100,
			})

			So(holds(Predicate{
				Subject: SubjectPosition, Strategy: "quick_pump", Op: ComparisonEquals,
			}, legacy), ShouldBeFalse)
		})
	})
}

func TestLatchExpiresOnContextClock(t *testing.T) {
	Convey("Given a compression-then-ignition sequence with a 90s latch TTL", t, func() {
		base := time.Unix(1_000_000, 0)
		thoughts := []Thought{{
			Name: "quick_pump",
			When: Predicate{
				Subject: SubjectSignal, Category: "coiled_compression",
				Unit: UnitSNR, Op: ComparisonAtLeast, Value: 0.9,
			},
			Then: []Thought{{
				Name: "ignite",
				When: Predicate{
					Subject: SubjectSignal, Category: "vertical_ignition",
					Unit: UnitSNR, Op: ComparisonAtLeast, Value: 0.9,
				},
				Do: Act{Type: ActionMarket, Fraction: 0.5},
			}},
		}}

		compression := map[types.CategoryType][]float64{"coiled_compression": {1.0}}
		ignition := map[types.CategoryType][]float64{"vertical_ignition": {1.0}}

		Convey("Ignition 30s after the latched compression still fires", func() {
			state := NewReasonState()

			_, fired := EvaluateStateful(thoughts, mockReason{now: base, signal: compression}, state)
			So(fired, ShouldBeFalse) // parent latches, no action yet

			act, fired := EvaluateStateful(thoughts, mockReason{
				now: base.Add(30 * time.Second), signal: ignition,
			}, state)
			So(fired, ShouldBeTrue)
			So(act.Type, ShouldEqual, ActionMarket)
		})

		Convey("Ignition two minutes later finds the latch expired — no stale entry", func() {
			state := NewReasonState()

			_, fired := EvaluateStateful(thoughts, mockReason{now: base, signal: compression}, state)
			So(fired, ShouldBeFalse)

			_, fired = EvaluateStateful(thoughts, mockReason{
				now: base.Add(2 * time.Minute), signal: ignition,
			}, state)
			So(fired, ShouldBeFalse)
		})
	})
}
