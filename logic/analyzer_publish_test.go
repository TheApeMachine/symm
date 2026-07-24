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
TestWireManifoldPrefersFocusCarrier proves the pilot-wave batch keeps shared
ρ/|ψ|² on the focused symbol in a single row so one UI frame paints the field.
*/
func TestWireManifoldPrefersFocusCarrier(t *testing.T) {
	Convey("Given three manifold states with lattices on ETH", t, func() {
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
				Source: "manifold",
				Symbol: "BTC/USD",
				At:     time.Unix(1, 0).UTC(),
			},
			{
				Source: "manifold",
				Symbol: "SOL/USD",
				At:     time.Unix(1, 0).UTC(),
			},
		}

		Convey("When focus is BTC/USD", func() {
			types.SetFocus("BTC/USD")
			Reset(func() { types.SetFocus("") })

			wired := wireManifold(states)
			So(wired, ShouldHaveLength, 1)
			So(wired[0].Symbol, ShouldEqual, "BTC/USD")
			So(wired[0].Rho, ShouldResemble, [][]float64{{0.1, 0.2}})
			So(wired[0].PsiMag2, ShouldResemble, [][]float64{{1, 0}})
			So(wired[0].Particles, ShouldBeNil)
		})

		Convey("When focus is unset", func() {
			types.SetFocus("")

			wired := wireManifold(states)
			So(wired, ShouldHaveLength, 1)
			So(wired[0].Symbol, ShouldEqual, "ETH/USD")
			So(wired[0].Rho, ShouldNotBeEmpty)
			So(wired[0].Particles, ShouldNotBeEmpty)
		})
	})
}

/*
TestPublishMeasuredEmitsOrderedFrames proves resonance precedes the single
focus-aware manifold frame so a saturated UI channel cannot starve charts.
*/
func TestPublishMeasuredEmitsOrderedFrames(t *testing.T) {
	Convey("Given resonance plus three manifold states", t, func() {
		ui := make(chan []byte, 8)
		analyzer := &Analyzer{ui: ui}
		thesis := types.NewThesis()
		thesis.Tick = 3
		thesis.Resonance = []any{
			&ResonanceOutcome{Source: "resonance", Symbol: "ETH/USD", Samples: 4},
		}
		states := []manifold.State{
			{
				Source:  "manifold",
				Symbol:  "ETH/USD",
				At:      time.Unix(1, 0).UTC(),
				Rho:     [][]float64{{0.1, 0.2}},
				PsiMag2: [][]float64{{1, 0}},
			},
			{
				Source: "manifold",
				Symbol: "BTC/USD",
				At:     time.Unix(1, 0).UTC(),
			},
		}

		types.SetFocus("BTC/USD")
		Reset(func() { types.SetFocus("") })

		analyzer.publishMeasured(thesis, states)

		Convey("It emits resonance then one manifold frame for focus", func() {
			So(len(ui), ShouldEqual, 2)

			resonance := string(<-ui)
			So(resonance, ShouldContainSubstring, `"resonance":`)
			So(resonance, ShouldContainSubstring, `"ETH/USD"`)

			manifoldFrame := string(<-ui)
			So(manifoldFrame, ShouldContainSubstring, `"manifold":[`)
			So(manifoldFrame, ShouldContainSubstring, `"BTC/USD"`)
			So(manifoldFrame, ShouldContainSubstring, `"rho":[[0.1,0.2]]`)
			So(manifoldFrame, ShouldNotContainSubstring, `"ETH/USD"`)
		})
	})
}

/*
TestPublishCognitionProjectsAllWinners proves strategy categories are not
focus-gated even when the cognition UI frame is.
*/
func TestPublishCognitionProjectsAllWinners(t *testing.T) {
	Convey("Given two ready cognition winners and a BTC focus", t, func() {
		ui := make(chan []byte, 8)
		analyzer := &Analyzer{ui: ui}
		thesis := types.NewThesis()
		thesis.Cognition.Store("ETH/USD", types.Cognition{
			Source: "dmt", Symbol: "ETH/USD", Winner: "buy", Ready: true,
			Confidence: 0.8, EntropyBits: 1, LookaheadScore: 0.5, Cohort: 2,
		})
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Source: "dmt", Symbol: "BTC/USD", Winner: "sell", Ready: true,
			Confidence: 0.6, EntropyBits: 2, LookaheadScore: 0.3, Cohort: 1,
		})

		types.SetFocus("BTC/USD")
		Reset(func() { types.SetFocus("") })

		analyzer.publishCognition(thesis)

		Convey("Thesis categories cover every ready winner", func() {
			So(thesis.Categories, ShouldHaveLength, 2)
		})

		Convey("UI cognition frame is focus-only", func() {
			So(len(ui), ShouldBeGreaterThanOrEqualTo, 1)
			frame := string(<-ui)
			So(frame, ShouldContainSubstring, `"cognition":`)
			So(frame, ShouldContainSubstring, `"BTC/USD"`)
			So(frame, ShouldNotContainSubstring, `"ETH/USD"`)
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
BenchmarkPublishMeasured measures the focus-aware single-frame manifold path.
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
