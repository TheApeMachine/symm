package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestCausalUpdate(t *testing.T) {
	Convey("Given a finite manifold state for a named causal model", t, func() {
		causal := NewCausal("BTC/USD")
		state := causalState(time.Unix(1, 0), 100, 1)

		hypothesis, outcome, err := causal.Update(state)

		Convey("It should return a durable hypothesis with discriminating provenance", func() {
			So(err, ShouldBeNil)
			So(outcome, ShouldNotBeNil)
			So(hypothesis.Source, ShouldEqual, types.SourceCausal)
			So(hypothesis.Symbol, ShouldEqual, "BTC/USD")
			So(hypothesis.Claim, ShouldEqual, causalHypothesis)
			So(hypothesis.Treatment, ShouldEqual, "bid_ask_touch_mass_imbalance")
			So(hypothesis.Controls, ShouldHaveLength, len(causalControls))
			So(hypothesis.Outcome, ShouldEqual, "next_l3_epoch_mid_log_return")
		})
	})
}

func TestCausalUpdateRejectsRegressions(t *testing.T) {
	Convey("Given a causal model holding a pending manifold state", t, func() {
		causal := NewCausal("BTC/USD")
		_, outcome, err := causal.Update(causalState(time.Unix(2, 0), 100, 2))
		So(err, ShouldBeNil)
		So(outcome, ShouldNotBeNil)

		_, duplicateOutcome, err := causal.Update(
			causalState(time.Unix(2, 0), 100, 2),
		)
		So(err, ShouldNotBeNil)

		_, sameTimeOutcome, err := causal.Update(
			causalState(time.Unix(2, 0), 101, 3),
		)
		So(err, ShouldBeNil)

		var regressedOutcome *CausalOutcome

		_, regressedOutcome, err = causal.Update(
			causalState(time.Unix(1, 0), 99, 1),
		)

		Convey("It should accept equal-time progress and reject duplicate or backward epochs", func() {
			So(err, ShouldNotBeNil)
			So(duplicateOutcome, ShouldBeNil)
			So(sameTimeOutcome, ShouldNotBeNil)
			So(regressedOutcome, ShouldBeNil)
			So(causal.pending.epoch, ShouldEqual, uint64(3))
			So(causal.pending.at, ShouldEqual, time.Unix(2, 0))
		})
	})
}

func BenchmarkCausalUpdate(b *testing.B) {
	causal := NewCausal("BTC/USD")
	b.ReportAllocs()

	for index := 1; b.Loop(); index++ {
		_, _, err := causal.Update(causalState(
			time.Unix(int64(index), 0),
			100+float64(index),
			uint64(index),
		))

		if err != nil {
			b.Fatal(err)
		}
	}
}

func causalState(at time.Time, midPrice float64, epoch uint64) manifold.State {
	state := manifold.State{
		Symbol:           "BTC/USD",
		At:               at,
		Epoch:            epoch,
		ReferencePrice:   midPrice,
		Spread:           0.01,
		BuyCapacity:      1000,
		SellCapacity:     1000,
		Duration:         time.Second,
		InvalidReason:    manifold.Valid,
		StressAnisotropy: 0.1,
		Subdivisions:     1,
		BuyIntensity:     0.6,
		SellIntensity:    0.4,
		SpectralRadius:   0.2,
	}
	state.Reading.PressureGradX = 0.2
	state.Reading.Divergence = 0.1
	state.Reading.CoherenceMag2 = 0.3
	state.Reading.GuidanceSpeed = 0.4

	return state
}
