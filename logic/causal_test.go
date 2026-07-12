package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
)

func TestCausalUpdate(t *testing.T) {
	Convey("Given a named causal hypothesis over future mid returns", t, func() {
		thesis := strategy.NewThesis()
		causal := NewCausal("BTC/USD", thesis, nil)
		thesis.AddEvidence("BTC/USD", "manifold", causalState(1, 100))

		causal.Update()
		first, ok := thesis.Evidence("BTC/USD", "causal")
		So(ok, ShouldBeTrue)

		Convey("It should expose the treatment, controls, and target while warming", func() {
			outcome := first.(CausalOutcome)
			So(outcome.Hypothesis, ShouldEqual, causalHypothesis)
			So(outcome.Treatment, ShouldEqual, "bid_ask_touch_mass_imbalance")
			So(outcome.Controls, ShouldHaveLength, len(causalControls))
			So(outcome.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
			So(outcome.Ready, ShouldBeFalse)
		})

		Convey("When the next epoch reveals the future target", func() {
			thesis.AddEvidence("BTC/USD", "manifold", causalState(2, 101))
			causal.Update()
			second, ok := thesis.Evidence("BTC/USD", "causal")

			Convey("Then it appends one aligned causal row", func() {
				So(ok, ShouldBeTrue)
				So(second.(CausalOutcome).Samples, ShouldEqual, uint64(1))
			})
		})
	})
}

func BenchmarkCausalUpdate(b *testing.B) {
	thesis := strategy.NewThesis()
	causal := NewCausal("BTC/USD", thesis, nil)

	for index := 1; b.Loop(); index++ {
		state := causalState(uint64(index), 100+float64(index%17)/100)
		state.PressureGradX = float64(index%7) / 10
		state.Divergence = float64(index%11) / 100
		state.StressAnisotropy = float64(index%13) / 100
		thesis.AddEvidence("BTC/USD", "manifold", state)
		causal.Update()
	}
}

func causalState(epoch uint64, midPrice float64) manifold.State {
	return manifold.State{
		Source:               "manifold",
		Symbol:               "BTC/USD",
		At:                   time.Unix(int64(epoch), 0),
		Epoch:                epoch,
		Ready:                true,
		BestBid:              midPrice - 0.5,
		BestAsk:              midPrice + 0.5,
		MidPrice:             midPrice,
		VisibleMass:          1,
		ConservationResidual: 0,
		BidTouchDensity:      0.6,
		AskTouchDensity:      0.4,
		StressAnisotropy:     0.1,
		DeltaT:               1,
		Subdivisions:         1,
		PriceScale:           1,
		SizeScale:            1,
		Reading: pmanifold.Reading{
			PressureGradX:    0.1,
			PressureGradNorm: 0.1,
			Divergence:       0.01,
			CoherenceMag2:    0.2,
			GuidanceSpeed:    0.03,
			ViscosityProxy:   0.1,
		},
	}
}
