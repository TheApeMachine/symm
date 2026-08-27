package advisor

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/types"
)

/*
singleCategory builds a one-category batch so a test can control the dominant
regime deterministically.
*/
func singleCategory(symbol string, at time.Time, categoryType types.CategoryType, strength float64) []types.Category {
	return []types.Category{
		{Symbol: symbol, At: at, Type: categoryType, Strength: strength},
	}
}

/*
feedSeries folds count alternating batches into an advisor and returns the last
perspective emitted.
*/
func feedSeries(advisor *HistoricalAnalogueAdvisor, symbol string, count int, categoryType types.CategoryType) *types.Perspective {
	var last *types.Perspective

	for index := 0; index < count; index++ {
		at := time.Unix(0, int64(index)*int64(time.Second))
		last = advisor.Step(singleCategory(symbol, at, categoryType, 1.0))
	}

	return last
}

func TestStep(t *testing.T) {
	Convey("Given a HistoricalAnalogueAdvisor", t, func() {
		advisor := NewHistoricalAnalogueAdvisor(t.Context(), nil)

		Convey("an empty batch produces no perspective", func() {
			So(advisor.Step(nil), ShouldBeNil)
			So(advisor.Step([]types.Category{}), ShouldBeNil)
		})

		Convey("the first batch emits context with no archive yet", func() {
			perspective := advisor.Step(singleCategory("TEST/USD", time.Unix(0, 0), types.VerticalIgnition, 1.0))

			So(perspective, ShouldNotBeNil)
			So(perspective.Symbol, ShouldEqual, "TEST/USD")
			So(perspective.Kind, ShouldEqual, types.KindHistoricalAnalogue)
			So(perspective.Analogue.Support, ShouldEqual, 0)
			So(perspective.Analogue.NearestDistance, ShouldEqual, 0)
			So(perspective.Analogue.MedianDistance, ShouldEqual, 0)
			So(perspective.Analogue.StageAlignment, ShouldBeGreaterThan, 0)
		})

		Convey("causal ordering: distances stay undefined until an archive and a comparable trajectory exist", func() {
			perspective := feedSeries(advisor, "TEST/USD", historicalTrajectoryLength+historicalMinComparable, types.VerticalIgnition)

			So(perspective, ShouldNotBeNil)
			So(perspective.Analogue.Support, ShouldBeGreaterThanOrEqualTo, 1)
			So(perspective.Analogue.NearestDistance, ShouldBeGreaterThanOrEqualTo, 0)
			So(perspective.Analogue.MedianDistance, ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("two advisors fed identical batches emit identical perspectives", func() {
			left := NewHistoricalAnalogueAdvisor(t.Context(), nil)
			right := NewHistoricalAnalogueAdvisor(t.Context(), nil)

			leftPerspective := feedSeries(left, "TEST/USD", 64, types.VerticalIgnition)
			rightPerspective := feedSeries(right, "TEST/USD", 64, types.VerticalIgnition)

			So(leftPerspective, ShouldNotBeNil)
			So(leftPerspective.Analogue.Support, ShouldEqual, rightPerspective.Analogue.Support)
			So(leftPerspective.Analogue.NearestDistance, ShouldEqual, rightPerspective.Analogue.NearestDistance)
			So(leftPerspective.Analogue.MedianDistance, ShouldEqual, rightPerspective.Analogue.MedianDistance)
			So(leftPerspective.Analogue.StageAlignment, ShouldEqual, rightPerspective.Analogue.StageAlignment)
			So(leftPerspective.Sequence, ShouldEqual, rightPerspective.Sequence)
		})

		Convey("resident state stays bounded regardless of how much history is folded in", func() {
			feedSeries(advisor, "TEST/USD", (historicalArchiveCapacity+5)*historicalTrajectoryLength, types.VerticalIgnition)

			slot := advisor.slotFor("TEST/USD")
			So(len(slot.archive), ShouldBeLessThanOrEqualTo, historicalArchiveCapacity)
		})
	})
}

func TestDominantRegime(t *testing.T) {
	Convey("Given a ranked category batch", t, func() {
		at := time.Unix(0, 0)

		Convey("the greatest-strength category is the regime", func() {
			batch := []types.Category{
				{Symbol: "TEST/USD", At: at, Type: types.CoiledCompression, Strength: 0.3},
				{Symbol: "TEST/USD", At: at, Type: types.VerticalIgnition, Strength: 0.9},
			}

			So(dominantRegime(batch), ShouldEqual, uint8(types.CategoryIndex(types.VerticalIgnition)))
		})

		Convey("categories without positive support are ignored", func() {
			batch := []types.Category{
				{Symbol: "TEST/USD", At: at, Type: types.VerticalIgnition, Strength: 0},
				{Symbol: "TEST/USD", At: at, Type: types.CoiledCompression, Strength: 0.5},
			}

			So(dominantRegime(batch), ShouldEqual, uint8(types.CategoryIndex(types.CoiledCompression)))
		})

		Convey("a batch with no supported category resolves to the zero regime", func() {
			batch := []types.Category{
				{Symbol: "TEST/USD", At: at, Type: types.VerticalIgnition, Strength: 0},
			}

			So(dominantRegime(batch), ShouldEqual, 0)
		})
	})
}

func TestNormalizedHamming(t *testing.T) {
	Convey("Given equal-length regime sequences", t, func() {
		Convey("identical sequences are distance zero", func() {
			So(normalizedHamming([]uint8{1, 2, 3}, []uint8{1, 2, 3}), ShouldEqual, 0)
		})

		Convey("fully disjoint sequences are distance one", func() {
			So(normalizedHamming([]uint8{1, 1}, []uint8{2, 2}), ShouldEqual, 1)
		})

		Convey("one mismatch of two is distance one half", func() {
			So(normalizedHamming([]uint8{1, 1}, []uint8{1, 2}), ShouldEqual, 0.5)
		})

		Convey("an empty sequence is distance zero", func() {
			So(normalizedHamming([]uint8{}, []uint8{}), ShouldEqual, 0)
		})
	})
}

func TestStageAlignment(t *testing.T) {
	Convey("Given an in-progress trajectory", t, func() {
		So(stageAlignment(0), ShouldEqual, 0)
		So(stageAlignment(historicalTrajectoryLength/2), ShouldEqual, 0.5)
		So(stageAlignment(historicalTrajectoryLength), ShouldEqual, 1)
	})
}

func TestAdvance(t *testing.T) {
	Convey("Given an empty resident slot", t, func() {
		slot := &resident{}
		now := time.Unix(0, 0)

		Convey("a full window archives and resets the in-progress trajectory", func() {
			for index := 0; index < historicalTrajectoryLength; index++ {
				slot.advance(1, now)
			}

			So(slot.fill, ShouldEqual, 0)
			So(len(slot.archive), ShouldEqual, 1)
			So(slot.archive[0][0], ShouldEqual, 1)

			Convey("the next observation starts a fresh window", func() {
				slot.advance(2, now)
				So(slot.fill, ShouldEqual, 1)
				So(slot.window[0], ShouldEqual, 2)
			})
		})
	})
}

func TestArchiveWindow(t *testing.T) {
	Convey("Given an archive at capacity", t, func() {
		slot := &resident{}

		for index := 0; index < historicalArchiveCapacity; index++ {
			slot.archive = append(slot.archive, [historicalTrajectoryLength]uint8{uint8(index)})
		}

		Convey("appending another window drops the oldest rather than growing", func() {
			slot.archiveWindow()

			So(len(slot.archive), ShouldEqual, historicalArchiveCapacity)
			So(slot.archive[0][0], ShouldEqual, 1)
			So(slot.archive[historicalArchiveCapacity-1][0], ShouldEqual, 0)
		})
	})
}

func TestDistanceSummary(t *testing.T) {
	Convey("Given a resident with a comparable trajectory and two archived windows", t, func() {
		slot := &resident{}
		slot.fill = 2
		slot.window[0] = 1
		slot.window[1] = 1

		slot.archive = append(slot.archive, [historicalTrajectoryLength]uint8{0, 0})
		slot.archive = append(slot.archive, [historicalTrajectoryLength]uint8{1, 1})

		Convey("the nearest and median distances are correct", func() {
			nearest, median := slot.distanceSummary()

			So(nearest, ShouldEqual, 0)
			So(median, ShouldEqual, 1)
		})
	})
}

/*
BenchmarkStep measures the steady-state cost of one perspective step after the
archive is warm. Allocation-free steady state is the target, not a hard gate.
*/
func BenchmarkStep(b *testing.B) {
	advisor := NewHistoricalAnalogueAdvisor(context.Background(), nil)
	feedSeries(advisor, "TEST/USD", 2*historicalTrajectoryLength, types.VerticalIgnition)

	at := time.Unix(0, int64(2*historicalTrajectoryLength)*int64(time.Second))
	batch := singleCategory("TEST/USD", at, types.VerticalIgnition, 1.0)

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		_ = advisor.Step(batch)
	}
}
