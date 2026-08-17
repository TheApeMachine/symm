package types

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationCount(t *testing.T) {
	Convey("Given thesis measurements grouped by source", t, func() {
		measurements := &sync.Map{}
		measurements.Store(SourceCVD, []*Measurement{{Symbol: "BTC/USD"}})
		measurements.Store(SourceLiquidity, []*Measurement{{Symbol: "ETH/USD"}, {Symbol: "BTC/USD"}})

		Convey("It counts distinct symbols across slice-backed source rows", func() {
			So(ObservationCount(measurements), ShouldEqual, 2)
		})
	})
}
