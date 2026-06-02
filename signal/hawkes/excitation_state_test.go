package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExcitationStateDecay(t *testing.T) {
	Convey("Given excitation sums", t, func() {
		state := ExcitationState{
			buyToBuy:   2,
			sellToBuy:  1,
			buyToSell:  1,
			sellToSell: 2,
			lastTime:   time.Now(),
			haveLast:   true,
		}
		later := state.lastTime.Add(time.Second)

		state.DecayTo(later, 1)

		Convey("It should decay excitation toward zero", func() {
			So(state.buyToBuy, ShouldBeLessThan, 2)
			So(state.lastTime, ShouldEqual, later)
		})
	})
}

func TestExcitationStateLogLikelihood(t *testing.T) {
	Convey("Given marked events", t, func() {
		start := time.Now()
		marked := []markedEvent{
			{at: start, side: sideBuy},
			{at: start.Add(time.Second), side: sideSell},
		}
		state := ExcitationState{}

		logSum, ok := state.LogLikelihoodSum(marked, 1, 1, 0.1, 0.1, 0.1, 0.1, 1)

		Convey("It should accumulate log likelihood", func() {
			So(ok, ShouldBeTrue)
			So(logSum, ShouldNotEqual, 0)
		})
	})
}
