package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestThesisAppendMeasurements(t *testing.T) {
	Convey("Given multiple measurements from one signal", t, func() {
		thesis := NewThesis(nil)
		measurements := []*Measurement{
			{Source: SourceLeadLag, Symbol: "BTC/USD", At: time.Unix(1, 0)},
			{Source: SourceLeadLag, Symbol: "ETH/USD", At: time.Unix(2, 0)},
			{Source: SourceLeadLag, Symbol: "SOL/USD", At: time.Unix(3, 0)},
		}

		thesis.AppendMeasurements(measurements, true)

		Convey("Then it should retain each measurement exactly once in source order", func() {
			stored, found := thesis.Measurements.Load(SourceLeadLag)
			So(found, ShouldBeTrue)
			actual := stored.([]*Measurement)
			So(actual, ShouldResemble, measurements)
			So(thesis.Readiness.LeadLag, ShouldBeTrue)
		})
	})
}
