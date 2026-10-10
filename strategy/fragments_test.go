package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestFragmentLog(t *testing.T) {
	Convey("Given a stored detection", t, func() {
		detection := data.NewMeasurement(7, "SUI/USD", "detector", 1, 30,
			&data.StringEntry{Key: "type", Value: "up"},
		)
		detection.Write(
			data.NewMetric("start_tick", 10, data.UnitCount, data.TimescaleTick),
			data.NewMetric("b_tick", 20, data.UnitCount, data.TimescaleTick),
			data.NewMetric("c_tick", 30, data.UnitCount, data.TimescaleTick),
			data.NewMetric("b_price", 2, data.UnitPrice, data.TimescaleTick),
			data.NewMetric("c_price", 3, data.UnitPrice, data.TimescaleTick),
		)

		log := &fragmentLog{}
		log.add(detection, []string{"R01", "R02"})

		Convey("It is listed with its A, B and C ticks as the marks the chart resolves", func() {
			So(log.fragments, ShouldHaveLength, 1)
			fragment := log.fragments[0]
			So(fragment.ID, ShouldEqual, 1)
			So(fragment.MarkA, ShouldEqual, 10)
			So(fragment.MarkB, ShouldEqual, 20)
			So(fragment.MarkC, ShouldEqual, 30)
			So(fragment.EntryIdx, ShouldEqual, 20)
			So(fragment.ExitIdx, ShouldEqual, 30)
			So(fragment.PredictedEntryIdx, ShouldEqual, -1)
			So(fragment.Class, ShouldEqual, "up")
			So(fragment.Tokens, ShouldResemble, []string{"R01", "R02"})
			So(fragment.Points, ShouldNotBeNil)
			So(log.tapes[1], ShouldResemble, fragmentTape{epoch: 7, label: "SUI/USD", low: 10, high: 30})
		})
	})
}

func TestRegionMetrics(t *testing.T) {
	Convey("Region metrics report evidenced regions only, with the winner Step advanced on", t, func() {
		var regions store.Regions
		regions.Counts[2], regions.Brightness[2] = 3, 1.5
		regions.Counts[5], regions.Brightness[5] = 1, -0.25
		regions.Winner, regions.RunnerUp = 2, 5

		metrics := regionMetrics(regions)
		So(metrics["region_winner"], ShouldEqual, 2)
		So(metrics["region_brightness:R02"], ShouldEqual, 1.5)
		So(metrics["region_members:R05"], ShouldEqual, 1)
		So(metrics, ShouldNotContainKey, "region_brightness:R01")
		So(string(regions.Token()), ShouldEqual, "R02")
	})
}
