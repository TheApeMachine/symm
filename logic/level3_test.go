package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

func TestLevel3Apply(t *testing.T) {
	Convey("Given a manifold result carrying a durable forecast", t, func() {
		thesis := types.NewThesis(nil)
		level3 := &Level3{}
		forecast := types.Forecasts{
			Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		}

		level3.apply(thesis, "BTC/USD", manifold.ProcessResult{
			Forecast: &forecast,
		})

		Convey("It should append the forecast directly to the thesis", func() {
			So(thesis.Forecasts, ShouldResemble, []types.Forecasts{forecast})
		})
	})
}

func TestLevel3ApplyRetainsLogicEvidence(t *testing.T) {
	Convey("Given chronological manifold states reaching Level3", t, func() {
		thesis := types.NewThesis(nil)
		level3 := &Level3{
			resonances: map[string]*Resonance{
				"BTC/USD": NewResonance(
					"BTC/USD", manifold.DefaultBaselineHalflife,
				),
			},
			causals: map[string]*Causal{
				"BTC/USD": NewCausal("BTC/USD"),
			},
			replay: manifold.NewReplayRecorder(),
		}

		for index := 1; index <= 16; index++ {
			state := causalState(
				time.Unix(int64(index), 0),
				100+float64(index),
				uint64(index),
			)
			state.PressureGradX += float64(index) / 100
			level3.apply(thesis, "BTC/USD", manifold.ProcessResult{
				State: state, GasReady: true,
			})
		}

		Convey("It should retain resonance measurements and causal hypotheses", func() {
			So(thesis.Measurements, ShouldNotBeEmpty)
			So(thesis.Hypotheses, ShouldHaveLength, 16)
			So(thesis.Hypotheses[0].Claim, ShouldEqual, causalHypothesis)
		})
	})
}
