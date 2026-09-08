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

func TestDevelopmentContextDepth(t *testing.T) {
	Convey("Retained context is bounded by the model's addressable depth", t, func() {
		at := time.Unix(1000, 0)
		development := &Development{Symbol: "BTC/USD"}

		for index := range MaxTemporalContextDepth * 3 {
			development.History = append(development.History, Context{
				At:         at.Add(time.Duration(index) * time.Second),
				Conditions: []uint64{uint64(index)},
			})
		}
		last := development.History[len(development.History)-1]
		measurement := data.NewMeasurement[float64]("fixture", "BTC/USD", "signal", at, at)
		tokens := development.Context(at, []*data.Measurement[float64]{measurement})

		So(development.History, ShouldHaveLength, MaxTemporalContextDepth)
		So(tokens, ShouldHaveLength, MaxTemporalContextDepth*2)

		Convey("The most recent transitions are the ones retained", func() {
			So(development.History[len(development.History)-1], ShouldResemble, last)
			So(tokens[len(tokens)-1], ShouldEqual, last.Conditions[0])
		})
	})
}
