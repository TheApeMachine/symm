package logic

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
			So(hypothesis.Treatment, ShouldEqual, "buy_sell_arrival_intensity_imbalance")
			So(hypothesis.Controls, ShouldHaveLength, len(causalControls))
			So(hypothesis.Outcome, ShouldEqual, "next_l3_epoch_mid_log_return")
		})
	})
}

/*
TestCausalUpdateIdentifiesArrivalEffect proves the named treatment against a
controlled structural process whose next return is generated only by the prior
buy/sell arrival imbalance while the physical controls vary independently.
*/
func TestCausalUpdateIdentifiesArrivalEffect(t *testing.T) {
	Convey("Given next-epoch returns caused by prior arrival imbalance", t, func() {
		causal := NewCausal("BTC/USD")
		price := 100.0
		treatments := []float64{-0.8, 0.35, -0.2, 0.7, -0.55, 0.15, 0.5, -0.4}
		var outcome *CausalOutcome

		for index := 1; index <= 64; index++ {
			if index > 1 {
				prior := treatments[(index-2)%len(treatments)]
				price *= math.Exp(0.002 * prior)
			}

			treatment := treatments[(index-1)%len(treatments)]
			state := causalState(time.Unix(int64(index), 0), price, uint64(index))
			state.BuyIntensity = (1 + treatment) / 2
			state.SellIntensity = (1 - treatment) / 2
			state.Reading.PressureGradX = math.Sin(float64(index) * 0.37)
			state.Reading.Divergence = math.Cos(float64(index) * 0.53)
			state.StressAnisotropy = math.Abs(math.Sin(float64(index) * 0.29))
			state.Reading.CoherenceMag2 = math.Abs(math.Cos(float64(index) * 0.41))
			state.Reading.GuidanceSpeed = math.Abs(math.Sin(float64(index) * 0.67))
			_, measured, err := causal.Update(state)
			So(err, ShouldBeNil)

			if measured != nil && measured.Ready {
				outcome = measured
			}
		}

		Convey("Then the causal ladder recovers the positive treatment direction", func() {
			So(outcome, ShouldNotBeNil)
			So(outcome.Reading.Association, ShouldBeGreaterThan, 0)
			So(outcome.Reading.Intervention, ShouldBeGreaterThan, 0)
			So(outcome.Reading.DoExpectation, ShouldBeGreaterThan, 0)
			So(outcome.At, ShouldEqual, time.Unix(63, 0))
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
		So(err, ShouldBeNil)
		So(duplicateOutcome, ShouldBeNil)

		_, sameTimeOutcome, err := causal.Update(
			causalState(time.Unix(2, 0), 101, 3),
		)
		So(err, ShouldBeNil)

		skipped := NewCausal("ETH/USD")
		_, _, skippedErr := skipped.Update(causalState(time.Unix(1, 0), 100, 1))
		_, skippedOutcome, skippedErr := skipped.Update(causalState(time.Unix(3, 0), 102, 3))
		_, regressedOutcome, regressedErr := skipped.Update(
			causalState(time.Unix(2, 0), 101, 2),
		)

		_, resynced, err := causal.Update(
			causalState(time.Unix(1, 0), 99, 1),
		)

		Convey("It should no-op repeats, accept progress, and resync after re-admit", func() {
			So(err, ShouldBeNil)
			So(duplicateOutcome, ShouldBeNil)
			So(sameTimeOutcome, ShouldNotBeNil)
			So(skippedErr, ShouldBeNil)
			So(skippedOutcome, ShouldNotBeNil)
			So(regressedErr, ShouldNotBeNil)
			So(regressedOutcome, ShouldBeNil)
			So(skipped.pending.epoch, ShouldEqual, uint64(3))
			So(resynced, ShouldNotBeNil)
			So(causal.pending.epoch, ShouldEqual, uint64(1))
			So(causal.pending.at, ShouldEqual, time.Unix(1, 0))
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
		ReferencePrice:   decimal.NewFromFloat64(midPrice),
		Spread:           0.01,
		BuyCapacity:      decimal.NewFromInt64(1000),
		SellCapacity:     decimal.NewFromInt64(1000),
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
