package strategy

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestNewBoard(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewBoard is called", func() {
			board := NewBoard(context.Background())

			Convey("Board is ready for ranking", func() {
				So(board, ShouldNotBeNil)
				So(board.peers, ShouldNotBeNil)
			})
		})
	})
}

func TestBoardScore(t *testing.T) {
	Convey("Setup", t, func() {
		board := NewBoard(context.Background())
		sel := system.Cfg.Selection

		Convey("When symbol or envelope is missing", func() {
			envelope := runtime.NewEnvelope(nil)

			Convey("Score is negative infinity", func() {
				So(board.Score("", envelope, nil), ShouldEqual, math.Inf(-1))
				So(board.Score("A/USD", nil, nil), ShouldEqual, math.Inf(-1))
			})
		})

		Convey("When correlation and pumpdump readings are set", func() {
			envelope := runtime.NewEnvelope(nil)
			envelope.Correlation["A/USD"] = &equation.CohortOutput{PeakScore: 2.0, HerdScore: 1.0}
			envelope.Pumpdump["A/USD"] = &equation.IgnitionOutput{Precursor: 1.0, Compression: 0.8}
			envelope.Correlation["B/USD"] = &equation.CohortOutput{PeakScore: 0.1, HerdScore: 0.0}
			envelope.Pumpdump["B/USD"] = &equation.IgnitionOutput{Precursor: 0.1, Compression: 0.0}

			wantA := sel.WeightPeakScore*2.0 +
				sel.WeightPeakScore*0.5*1.0 +
				sel.WeightPrecursor*1.0 +
				sel.WeightPrecursor*0.5*0.8
			wantB := sel.WeightPeakScore*0.1 +
				sel.WeightPrecursor*0.1

			Convey("Score matches the weighted envelope terms", func() {
				So(board.Score("A/USD", envelope, nil), ShouldEqual, wantA)
				So(board.Score("B/USD", envelope, nil), ShouldEqual, wantB)
			})
		})
	})
}

func TestPlace(t *testing.T) {
	Convey("Setup", t, func() {
		board := NewBoard(context.Background())

		Convey("When two candidates compete for one enter slot", func() {
			var entered []string
			enter := func(cand EnterCandidate) {
				entered = append(entered, cand.Symbol)
			}

			envelope := runtime.NewEnvelope(nil)
			envelope.Correlation["TOP/USD"] = &equation.CohortOutput{PeakScore: 3.0}
			envelope.Correlation["LOW/USD"] = &equation.CohortOutput{PeakScore: 0.1}

			candidates := []EnterCandidate{
				{Symbol: "LOW/USD", Path: "p1"},
				{Symbol: "TOP/USD", Path: "p2"},
			}

			board.Place(candidates, envelope, nil, nil, false, enter)

			Convey("Only the higher-scoring symbol enters", func() {
				So(entered, ShouldResemble, []string{"TOP/USD"})
			})
		})

		Convey("When a lone candidate sits below the board floor", func() {
			var entered []string
			var skipped string
			enter := func(cand EnterCandidate) {
				entered = append(entered, cand.Symbol)
			}

			envelope := runtime.NewEnvelope(nil)
			envelope.Correlation["FLAT/USD"] = &equation.CohortOutput{PeakScore: 0.1}
			candidates := []EnterCandidate{{
				Symbol: "FLAT/USD",
				Path:   "p",
				Decide: func(d *runtime.Decision) {
					if d != nil {
						skipped = d.Reason
					}
				},
			}}

			board.Place(candidates, envelope, nil, nil, false, enter)

			Convey("Place skips instead of free 1-of-1 enter", func() {
				So(entered, ShouldBeEmpty)
				So(skipped, ShouldStartWith, "board score below floor")
			})
		})

		Convey("When enter callback is nil", func() {
			envelope := runtime.NewEnvelope(nil)
			candidates := []EnterCandidate{{Symbol: "X/USD", Path: "p"}}

			Convey("Place is a no-op", func() {
				So(func() { board.Place(candidates, envelope, nil, nil, false, nil) }, ShouldNotPanic)
			})
		})
	})
}

func BenchmarkNewBoard(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewBoard(ctx)
	}
}

func BenchmarkBoardScore(b *testing.B) {
	board := NewBoard(context.Background())
	envelope := runtime.NewEnvelope(nil)
	envelope.Correlation["A/USD"] = &equation.CohortOutput{PeakScore: 2.0, HerdScore: 1.0}
	envelope.Pumpdump["A/USD"] = &equation.IgnitionOutput{Precursor: 1.0, Compression: 0.8}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = board.Score("A/USD", envelope, nil)
	}
}

func BenchmarkPlace(b *testing.B) {
	board := NewBoard(context.Background())
	envelope := runtime.NewEnvelope(nil)
	envelope.Correlation["TOP/USD"] = &equation.CohortOutput{PeakScore: 3.0}
	envelope.Correlation["LOW/USD"] = &equation.CohortOutput{PeakScore: 0.1}
	candidates := []EnterCandidate{
		{Symbol: "LOW/USD", Path: "p1"},
		{Symbol: "TOP/USD", Path: "p2"},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		board.Place(candidates, envelope, nil, nil, false, func(EnterCandidate) {})
	}
}
