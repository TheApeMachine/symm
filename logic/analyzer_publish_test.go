package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
TestAnalyzerPublish proves saturated frames allocate nothing and direct manifold
state slices retain the established serialized payload exactly.
*/
func TestAnalyzerPublish(t *testing.T) {
	Convey("Given a saturated Analyzer publication channel", t, func() {
		ui := make(chan []byte, 1)
		occupied := []byte("occupied")
		ui <- occupied
		analyzer := &Analyzer{ui: ui}
		frame := datura.Map[any]{"manifold": []manifold.State{{
			Source: "manifold",
			Symbol: "BTC/USD",
		}}}

		Convey("It should drop without serializing or replacing queued data", func() {
			allocations := testing.AllocsPerRun(100, func() {
				analyzer.publish(frame)
			})

			So(allocations, ShouldEqual, 0)
			So(<-ui, ShouldResemble, occupied)
		})
	})

	Convey("Given measured manifold states ready for publication", t, func() {
		ui := make(chan []byte, 1)
		analyzer := &Analyzer{ui: ui}
		thesis := types.NewThesis(nil)
		states := []manifold.State{{
			Source: "manifold",
			Symbol: "BTC/USD",
		}}
		boxed := make([]any, len(states))

		for index := range states {
			boxed[index] = states[index]
		}

		expected := datura.Map[any]{"manifold": boxed}.Marshal()
		analyzer.publishMeasured(thesis, states)

		Convey("It should preserve the established serialized payload exactly", func() {
			So(<-ui, ShouldResemble, expected)
		})
	})
}

func TestProjectCategoriesFromCognition(t *testing.T) {
	Convey("Given ready cognition winners on a thesis", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis(nil)
		analyzer.projectCategories(thesis, []types.Cognition{
			{
				Symbol:         "PENGU/USD",
				At:             time.Unix(1, 0).UTC(),
				Winner:         "buy",
				Ready:          true,
				Confidence:     0.72,
				EntropyBits:    1.25,
				LookaheadScore: 0.4,
				Cohort:         3,
			},
			{
				Symbol: "SKIP/USD",
				Winner: "buy",
				Ready:  false,
			},
		})

		Convey("Then only ready winners become category rows", func() {
			So(len(thesis.Categories), ShouldEqual, 1)
			So(thesis.Categories[0].Symbol, ShouldEqual, "PENGU/USD")
			So(string(thesis.Categories[0].Type), ShouldEqual, "buy")
			So(thesis.Categories[0].Confidence, ShouldEqual, 0.72)
			So(thesis.Categories[0].Strength, ShouldEqual, 0.4)
		})
	})
}

func BenchmarkProjectCategories(b *testing.B) {
	analyzer := &Analyzer{}
	thesis := types.NewThesis(nil)
	cognition := []types.Cognition{
		{
			Symbol:         "PENGU/USD",
			At:             time.Unix(1, 0).UTC(),
			Winner:         "buy",
			Ready:          true,
			Confidence:     0.72,
			EntropyBits:    1.25,
			LookaheadScore: 0.4,
			Cohort:         3,
		},
		{
			Symbol: "SKIP/USD",
			Winner: "buy",
			Ready:  false,
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		thesis.Categories = nil
		analyzer.projectCategories(thesis, cognition)
	}
}

/*
BenchmarkAnalyzerPublish measures the saturated UI path that must drop a frame
without paying its serialization cost.
*/
func BenchmarkAnalyzerPublish(b *testing.B) {
	ui := make(chan []byte, 1)
	ui <- []byte("occupied")
	frame := datura.Map[any]{"manifold": []manifold.State{{
		Source: "manifold",
		Symbol: "BTC/USD",
	}}}
	analyzer := &Analyzer{ui: ui}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		analyzer.publish(frame)
	}
}
