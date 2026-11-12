package strategy

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/runtime"
)

func TestBoardScore(t *testing.T) {
	Convey("Board score favors high peak and precursor", t, func() {
		board := NewBoard(context.Background(), NewRotation(context.Background()))
		envelope := runtime.NewEnvelope(nil)

		envelope.Correlation["A/USD"] = &equation.CohortOutput{PeakScore: 2.0, HerdScore: 1.0}
		envelope.Pumpdump["A/USD"] = &equation.IgnitionOutput{Precursor: 1.0, Compression: 0.8}

		envelope.Correlation["B/USD"] = &equation.CohortOutput{PeakScore: 0.1, HerdScore: 0.0}
		envelope.Pumpdump["B/USD"] = &equation.IgnitionOutput{Precursor: 0.1, Compression: 0.0}

		So(board.Score("A/USD", envelope, nil), ShouldBeGreaterThan, board.Score("B/USD", envelope, nil))
	})
}

func TestBoardPlaceRanks(t *testing.T) {
	Convey("Board place keeps only top enter per tick", t, func() {
		board := NewBoard(context.Background(), nil)
		var entered []string

		place := func(cand EnterCandidate) {
			entered = append(entered, cand.Symbol)
		}

		envelope := runtime.NewEnvelope(nil)
		envelope.Correlation["TOP/USD"] = &equation.CohortOutput{PeakScore: 3.0}
		envelope.Correlation["LOW/USD"] = &equation.CohortOutput{PeakScore: 0.1}

		candidates := []EnterCandidate{
			{Symbol: "LOW/USD", Path: "p1"},
			{Symbol: "TOP/USD", Path: "p2"},
		}

		board.Place(candidates, envelope, nil, nil, false, place)

		So(entered, ShouldResemble, []string{"TOP/USD"})
	})
}
