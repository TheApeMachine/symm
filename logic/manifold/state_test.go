package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStateSpreadReturn(t *testing.T) {
	Convey("Given a typed manifold state with an executable spread", t, func() {
		state := State{BestBid: 99, BestAsk: 101, MidPrice: 100}

		Convey("It should report spread in return units", func() {
			So(state.HasSpread(), ShouldBeTrue)
			So(state.SpreadReturn(), ShouldAlmostEqual, 0.02)
		})
	})
}

func TestStateMarshalJSON(t *testing.T) {
	Convey("Given a typed manifold state", t, func() {
		state := State{
			FieldSnapshot: FieldSnapshot{Rho: [][]float64{{1, 2}}},
			Source:        "manifold",
			Symbol:        "BTC/USD",
			At:            time.Unix(1, 0).UTC(),
			Epoch:         2,
			MidPrice:      100,
		}

		frame, err := sonic.Marshal(state)
		So(err, ShouldBeNil)
		decoded := map[string]any{}
		err = sonic.Unmarshal(frame, &decoded)

		Convey("It should preserve domain fields without a UI DTO", func() {
			So(err, ShouldBeNil)
			So(decoded["source"], ShouldEqual, "manifold")
			So(decoded["symbol"], ShouldEqual, "BTC/USD")
			So(decoded["epoch"], ShouldEqual, float64(2))
			So(decoded["midPrice"], ShouldEqual, float64(100))
			So(decoded["rho"], ShouldNotBeNil)
		})
	})
}

func TestStateGasReadyConservationBound(t *testing.T) {
	Convey("Given a finite state at its derived conservation boundary", t, func() {
		bound := math.Nextafter(1, math.Inf(1)) - 1
		state := State{
			Ready: true, VisibleMass: 1,
			ConservationResidual: bound, ConservationBound: bound,
			DeltaT: 1, Subdivisions: 1, PriceScale: 1, SizeScale: 1,
		}

		Convey("It should accept equality with the bound", func() {
			So(state.GasReady(), ShouldBeTrue)
		})

		Convey("It should reject the next representable residual", func() {
			state.ConservationResidual = math.Nextafter(bound, math.Inf(1))
			So(state.GasReady(), ShouldBeFalse)
		})

		Convey("It should reject a non-finite bound", func() {
			state.ConservationBound = math.NaN()
			So(state.GasReady(), ShouldBeFalse)
		})
	})
}
