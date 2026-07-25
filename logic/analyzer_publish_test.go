package logic

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestCalibrateCategories(t *testing.T) {
	Convey("Given composed categories and ready cognition", t, func() {
		analyzer := &Analyzer{}
		thesis := types.NewThesis()
		thesis.Categories = []types.Category{
			{
				Symbol: "PENGU/USD", Type: types.VerticalIgnition,
				Strength: 0.5, Confidence: 0.2,
			},
			{
				Symbol: "PENGU/USD", Type: types.OrganicTrend,
				Strength: 0.8, Confidence: 0.3,
			},
		}
		analyzer.calibrateCategories(thesis, []types.Cognition{
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

		Convey("Then DMT confidence stamps the strongest composed row", func() {
			So(len(thesis.Categories), ShouldEqual, 2)
			So(thesis.Categories[1].Type, ShouldEqual, types.OrganicTrend)
			So(thesis.Categories[1].Confidence, ShouldEqual, 0.72)
			So(thesis.Categories[1].Surprisal, ShouldEqual, 1.25)
			So(thesis.Categories[0].Confidence, ShouldEqual, 0.2)
		})
	})
}

/*
TestWireManifoldPrefersFocusCarrier proves the pilot-wave batch keeps the shared
GPU display texture on the focused symbol in a single row so one UI frame paints
the field.
*/
func TestWireManifoldPrefersFocusCarrier(t *testing.T) {
	Convey("Given three manifold states with display on ETH", t, func() {
		display := []byte{10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 100, 110, 120, 255}
		states := []manifold.State{
			{
				Source:          "manifold",
				Symbol:          "ETH/USD",
				At:              time.Unix(1, 0).UTC(),
				Display:         display,
				DisplayWidth:    2,
				DisplayHeight:   2,
				OscillatorCount: 3,
				RhoOccupied:     2,
				PsiOccupied:     2,
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
			So(wired[0].Display, ShouldResemble, display)
			So(wired[0].DisplayWidth, ShouldEqual, 2)
			So(wired[0].OscillatorCount, ShouldEqual, 3)
		})

		Convey("When focus is unset", func() {
			types.SetFocus("")

			wired := wireManifold(states)
			So(wired, ShouldHaveLength, 1)
			So(wired[0].Symbol, ShouldEqual, "ETH/USD")
			So(wired[0].Display, ShouldNotBeEmpty)
			So(wired[0].OscillatorCount, ShouldEqual, 3)
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
			&ResonanceOutcome{Source: "resonance", Symbol: "BTC/USD", Samples: 8},
		}
		thesis.Causal = []any{
			&CausalOutcome{Source: "causal", Symbol: "ETH/USD", Ready: true},
			&CausalOutcome{Source: "causal", Symbol: "BTC/USD", Ready: true},
		}
		thesis.Hypotheses = []types.Hypothesis{
			{Source: "causal", Symbol: "ETH/USD", Claim: "eth"},
			{Source: "causal", Symbol: "BTC/USD", Claim: "btc"},
		}
		states := []manifold.State{
			{
				Source:        "manifold",
				Symbol:        "ETH/USD",
				At:            time.Unix(1, 0).UTC(),
				Display:       []byte{10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 100, 110, 120, 255},
				DisplayWidth:  2,
				DisplayHeight: 2,
				RhoOccupied:   1,
				PsiOccupied:   1,
			},
			{
				Source: "manifold",
				Symbol: "BTC/USD",
				At:     time.Unix(1, 0).UTC(),
			},
		}

		types.SetFocus("BTC/USD")
		Reset(func() { types.SetFocus("") })

		analyzer.publishMeasured(thesis, states, 0, thesis.Tick)

		Convey("Thesis rows stay book-wide for strategy", func() {
			So(thesis.Resonance, ShouldHaveLength, 2)
			So(thesis.Causal, ShouldHaveLength, 2)
			So(thesis.Hypotheses, ShouldHaveLength, 2)
		})

		Convey("It emits focus-only rows, meta, one display texture, wave", func() {
			// resonance + causal + hypotheses + manifold meta + display + wave.
			So(len(ui), ShouldEqual, 6)

			resonance := string(<-ui)
			So(resonance, ShouldContainSubstring, `"resonance":`)
			So(resonance, ShouldContainSubstring, `"BTC/USD"`)
			So(resonance, ShouldNotContainSubstring, `"ETH/USD"`)

			causal := string(<-ui)
			So(causal, ShouldContainSubstring, `"causal":`)
			So(causal, ShouldContainSubstring, `"BTC/USD"`)
			So(causal, ShouldNotContainSubstring, `"ETH/USD"`)

			hypotheses := string(<-ui)
			So(hypotheses, ShouldContainSubstring, `"hypotheses":`)
			So(hypotheses, ShouldContainSubstring, `"BTC/USD"`)
			So(hypotheses, ShouldNotContainSubstring, `"ETH/USD"`)

			manifoldFrame := string(<-ui)
			So(manifoldFrame, ShouldContainSubstring, `"manifold":[`)
			So(manifoldFrame, ShouldContainSubstring, `"BTC/USD"`)
			So(manifoldFrame, ShouldNotContainSubstring, `"rho"`)
			So(manifoldFrame, ShouldNotContainSubstring, `"ETH/USD"`)

			display := <-ui
			key, ok := manifold.BinaryCacheKey(display)
			So(ok, ShouldBeTrue)
			So(key, ShouldEqual, "manifold_display")

			waveFrame := string(<-ui)
			So(waveFrame, ShouldContainSubstring, `"manifold_wave":`)
			So(waveFrame, ShouldContainSubstring, `"BTC/USD"`)
			So(waveFrame, ShouldNotContainSubstring, `"manifold_particles"`)
		})
	})
}

/*
TestPublishCognitionProjectsAllWinners proves thesis categories stay book-wide
for strategy while cognition and category UI frames are focus-gated.
*/
func TestPublishCognitionProjectsAllWinners(t *testing.T) {
	Convey("Given two ready cognition winners and a BTC focus", t, func() {
		ui := make(chan []byte, 8)
		analyzer := &Analyzer{ui: ui}
		thesis := types.NewThesis()
		thesis.Categories = []types.Category{
			{
				Symbol: "ETH/USD", Type: types.VerticalIgnition,
				Strength: 0.5, Confidence: 0.2,
			},
			{
				Symbol: "BTC/USD", Type: types.OrganicTrend,
				Strength: 0.8, Confidence: 0.3,
			},
		}
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

		Convey("Thesis categories stay book-wide for strategy", func() {
			So(thesis.Categories, ShouldHaveLength, 2)
			So(thesis.Categories[0].Symbol, ShouldEqual, "ETH/USD")
			So(thesis.Categories[1].Symbol, ShouldEqual, "BTC/USD")
		})

		Convey("UI cognition and categories frames are focus-only", func() {
			So(len(ui), ShouldEqual, 2)

			cognitionFrame := string(<-ui)
			So(cognitionFrame, ShouldContainSubstring, `"cognition":`)
			So(cognitionFrame, ShouldContainSubstring, `"BTC/USD"`)
			So(cognitionFrame, ShouldNotContainSubstring, `"ETH/USD"`)

			categoriesFrame := string(<-ui)
			So(categoriesFrame, ShouldContainSubstring, `"categories":`)
			So(categoriesFrame, ShouldContainSubstring, `"BTC/USD"`)
			So(categoriesFrame, ShouldContainSubstring, `"organic_trend"`)
			So(categoriesFrame, ShouldNotContainSubstring, `"ETH/USD"`)
			So(categoriesFrame, ShouldNotContainSubstring, `"vertical_ignition"`)
		})
	})
}

/*
TestFocusCategories keeps the book-wide slice when focus is unset and collapses
to the focused symbol otherwise.
*/
func TestFocusCategories(t *testing.T) {
	Convey("Given categories for two symbols", t, func() {
		categories := []types.Category{
			{Symbol: "ETH/USD", Type: types.VerticalIgnition, Strength: 0.5},
			{Symbol: "BTC/USD", Type: types.OrganicTrend, Strength: 0.8},
			{Symbol: "ETH/USD", Type: types.DenseNeutrality, Strength: 0.3},
		}

		Convey("When focus is unset", func() {
			types.SetFocus("")
			Reset(func() { types.SetFocus("") })

			So(focusCategories(categories), ShouldHaveLength, 3)
		})

		Convey("When focus is BTC", func() {
			types.SetFocus("BTC/USD")
			Reset(func() { types.SetFocus("") })

			framed := focusCategories(categories)
			So(framed, ShouldHaveLength, 1)
			So(framed[0].Symbol, ShouldEqual, "BTC/USD")
			So(framed[0].Type, ShouldEqual, types.OrganicTrend)
		})
	})
}

/*
TestFocusRows collapses resonance/causal any-rows to the focused symbol.
*/
func TestFocusRows(t *testing.T) {
	Convey("Given resonance rows for two symbols", t, func() {
		rows := []any{
			&ResonanceOutcome{Symbol: "ETH/USD", Samples: 1},
			ResonanceOutcome{Symbol: "BTC/USD", Samples: 2},
			&CausalOutcome{Symbol: "ETH/USD"},
		}

		Convey("When focus is unset", func() {
			types.SetFocus("")
			Reset(func() { types.SetFocus("") })

			So(focusRows(rows), ShouldHaveLength, 3)
		})

		Convey("When focus is BTC", func() {
			types.SetFocus("BTC/USD")
			Reset(func() { types.SetFocus("") })

			framed := focusRows(rows)
			So(framed, ShouldHaveLength, 1)
			So(rowSymbol(framed[0]), ShouldEqual, "BTC/USD")
		})
	})
}

func BenchmarkCalibrateCategories(b *testing.B) {
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
		thesis.Categories = []types.Category{{
			Symbol: "PENGU/USD", Type: types.VerticalIgnition, Strength: 0.5,
		}}
		analyzer.calibrateCategories(thesis, cognition)
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

	states[0].Display = []byte{10, 20, 30, 255, 40, 50, 60, 255, 70, 80, 90, 255, 100, 110, 120, 255}
	states[0].DisplayWidth = 2
	states[0].DisplayHeight = 2

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

		analyzer.publishMeasured(thesis, states, 0, thesis.Tick)
	}
}
