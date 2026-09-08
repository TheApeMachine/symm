package strategy

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
	"time"
)

func TestDevelopmentContext(t *testing.T) {
	Convey("A producer horizon selects ordered prior grid conditions", t, func() {
		at := time.Unix(100, 0)
		development := &Development{History: []Context{
			{At: at.Add(-3 * time.Second), Conditions: []uint64{10}},
			{At: at.Add(-2 * time.Second), Conditions: []uint64{20, 21}},
			{At: at.Add(-time.Second), Conditions: []uint64{30}},
		}}
		measurement := data.NewMeasurement[float64]("fixture", "BTC/USD", "signal", at, at.Add(-2*time.Second))
		tokens := development.Context(at, []*data.Measurement[float64]{measurement})
		So(tokens, ShouldResemble, []uint64{uint64(1)<<63 | 2, 20, 21, uint64(1)<<63 | 2, 30})
		So(development.History, ShouldHaveLength, 2)
		So(development.From, ShouldResemble, measurement.From)

		Convey("A later horizon releases older conditions", func() {
			measurement.From = at
			So(development.Context(at, []*data.Measurement[float64]{measurement}), ShouldBeEmpty)
			So(development.History, ShouldBeEmpty)
		})
	})
}
