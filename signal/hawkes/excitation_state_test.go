package hawkes

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExcitationStateDecayTo(t *testing.T) {
	Convey("Given excitation sums at an earlier time", t, func() {
		state := &ExcitationState{
			buyToBuy:   2,
			sellToBuy:  1,
			buyToSell:  1,
			sellToSell: 2,
			lastTime:   time.Unix(100, 0),
			haveLast:   true,
		}
		start := state.buyToBuy

		state.DecayTo(time.Unix(101, 0), 1.0)

		Convey("It should decay all branches toward zero", func() {
			So(state.buyToBuy, ShouldBeLessThan, start)
			So(state.sellToSell, ShouldBeLessThan, start)
		})
	})
}

func TestExcitationStateLogLikelihoodSum(t *testing.T) {
	Convey("Given a short marked buy burst", t, func() {
		start := time.Unix(1_700_000_000, 0)
		marked := []markedEvent{
			{at: start, side: sideBuy},
			{at: start.Add(time.Second), side: sideBuy},
		}

		logSum, ok := (&ExcitationState{}).LogLikelihoodSum(
			marked,
			0.5, 0.5,
			0.2, 0.1, 0.1, 0.2,
			1.0,
		)

		Convey("It should accumulate a finite log-likelihood", func() {
			So(ok, ShouldBeTrue)
			So(math.IsNaN(logSum), ShouldBeFalse)
			So(math.IsInf(logSum, 0), ShouldBeFalse)
		})
	})

	Convey("Given an empty event set", t, func() {
		_, ok := (&ExcitationState{}).LogLikelihoodSum(nil, 1, 1, 1, 1, 1, 1, 1)

		Convey("It should reject the estimate", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func BenchmarkExcitationStateDecayTo(b *testing.B) {
	state := &ExcitationState{
		buyToBuy: 2,
		lastTime: time.Unix(100, 0),
		haveLast: true,
	}
	at := time.Unix(101, 0)

	for b.Loop() {
		state.DecayTo(at, 1.0)
	}
}
