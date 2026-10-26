package morphology

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCalculateBookLayout(t *testing.T) {
	Convey("Calculate reads best bid and ask from the shared prefix", t, func() {
		signal := &Signal{states: make(map[string]*symbolState)}

		bids := []float64{
			100.0, 101.0,
			99.5, 2.0,
			99.0, 1.0,
		}
		asks := []float64{
			100.0, 101.0,
			101.5, 2.0,
			102.0, 1.0,
		}

		first, err := signal.Calculate("BTC/USD", bids, asks)
		So(err, ShouldBeNil)
		So(len(first), ShouldEqual, 6)

		second, err := signal.Calculate("BTC/USD", bids, asks)
		So(err, ShouldBeNil)
		So(len(second), ShouldEqual, 7)
		So(second[6], ShouldBeGreaterThanOrEqualTo, 0)
	})
}
