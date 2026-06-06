package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

// mockReason answers predicate queries from ago-indexed series (index 0 = now).
type mockReason struct {
	regime    types.Regime
	lifecycle map[types.ObservationType]bool
	signal    map[types.CategoryType][]float64
	price     []float64
	volume    []float64
	elapsed   float64
}

func (m mockReason) Regime() types.Regime                   { return m.regime }
func (m mockReason) PositionSide() trading.Side             { return "" }
func (m mockReason) Lifecycle(s types.ObservationType) bool { return m.lifecycle[s] }

func (m mockReason) Signal(c types.CategoryType, _ UnitType, ago int) (float64, bool) {
	series, ok := m.signal[c]
	if !ok || ago < 0 || ago >= len(series) {
		return 0, false
	}
	return series[ago], true
}

func (m mockReason) Scalar(subject Subject, _ UnitType, ago int) (float64, bool) {
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
	if ago < 0 || ago >= len(series) {
		return 0, false
	}
	return series[ago], true
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
