package logic

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestProjectCategoriesFromCognition(t *testing.T) {
	Convey("Given ready cognition winners on a thesis", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis()
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

/*
TestPublishMeasuredSplitsManifold proves each symbol is its own UI frame so the
worker never clones a full-cohort manifold payload in one DRAW.
*/
func TestPublishMeasuredSplitsManifold(t *testing.T) {
	Convey("Given three manifold states with lattices on the first", t, func() {
		ui := make(chan []byte, 8)
		analyzer := &Analyzer{ui: ui}
		thesis := types.NewThesis()
		thesis.Tick = 3
		states := []manifold.State{
			{
				Source:    "manifold",
				Symbol:    "ETH/USD",
				At:        time.Unix(1, 0).UTC(),
				Rho:       [][]float64{{0.1, 0.2}},
				PsiMag2:   [][]float64{{1, 0}},
				Particles: []manifold.Particle{{Role: "particle"}},
			},
			{
				Source:  "manifold",
				Symbol:  "BTC/USD",
				At:      time.Unix(1, 0).UTC(),
				Rho:     [][]float64{{0.3, 0.4}},
				PsiMag2: [][]float64{{0, 1}},
			},
			{
				Source: "manifold",
				Symbol: "SOL/USD",
				At:     time.Unix(1, 0).UTC(),
			},
		}

		Convey("When publishMeasured runs", func() {
			analyzer.publishMeasured(thesis, states)

			Convey("It emits one manifold frame per symbol", func() {
				So(len(ui), ShouldEqual, 3)

				for range 3 {
					payload := <-ui
					So(string(payload), ShouldContainSubstring, `"manifold":[`)
					So(string(payload), ShouldNotContainSubstring, `"BTC/USD","SOL/USD"`)
				}
			})

			Convey("It keeps shared lattices only on the carrier row", func() {
				wired := wireManifold(states)
				So(wired[0].Rho, ShouldNotBeEmpty)
				So(wired[0].Particles, ShouldNotBeEmpty)
				So(wired[1].Rho, ShouldBeNil)
				So(wired[1].Particles, ShouldBeNil)
				So(wired[2].Rho, ShouldBeNil)
			})
		})
	})
}

func BenchmarkProjectCategories(b *testing.B) {
	analyzer := &Analyzer{}
	thesis := types.NewThesis()
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
BenchmarkPublishMeasured measures one-symbol-per-frame manifold fan-out.
*/
func BenchmarkPublishMeasured(b *testing.B) {
	ui := make(chan []byte, 256)
	analyzer := &Analyzer{ui: ui}
	thesis := types.NewThesis()
	states := make([]manifold.State, 32)

	for index := range states {
		states[index] = manifold.State{
			Source: "manifold",
			Symbol: fmt.Sprintf("S%d/USD", index),
			At:     time.Unix(int64(index+1), 0).UTC(),
		}
	}

	states[0].Rho = [][]float64{{0.1, 0.2}, {0.3, 0.4}}
	states[0].PsiMag2 = [][]float64{{1, 0}, {0, 1}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for draining := true; draining; {
			select {
			case <-ui:
			default:
				draining = false
			}
		}

		analyzer.publishMeasured(thesis, states)
	}
}
