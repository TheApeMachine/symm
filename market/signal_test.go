package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

type signalStub struct{}

func (signalStub) Measure(_ perspectives.Feedback) types.Measurement {
	return types.Measurement{Symbol: "BTC/EUR"}
}

type feedbackStub struct{}

func (feedbackStub) MSE() float64   { return 0 }
func (feedbackStub) Scale() float64 { return 1 }
func (feedbackStub) Bias() float64  { return 0 }
func (feedbackStub) Samples() int   { return 1 }

func TestSignalMeasure(t *testing.T) {
	Convey("Given a Signal implementation", t, func() {
		var signal Signal = signalStub{}

		Convey("It should return measurements from Measure", func() {
			reading := signal.Measure(feedbackStub{})

			So(reading.Symbol, ShouldEqual, "BTC/EUR")
		})
	})
}
