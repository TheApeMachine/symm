package store

import (
	"context"
	"maps"
	"math"
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
)

/*
pinned returns one real (source, label, region) pin, so fixtures use the
grid's declared keys.
*/
func pinned() (string, string, uint8) {
	source := slices.Sorted(maps.Keys(regionMetricPins))[0]
	label := slices.Sorted(maps.Keys(regionMetricPins[source]))[0]

	return source, label, regionMetricPins[source][label][0]
}

func tick(symbol, source, label string, value float64) *data.Value {
	frame := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	data.AppendSignalVector(frame, source, symbol, []float64{value}, label)

	return frame
}

func TestSNM(t *testing.T) {
	Convey("SNM is bounded self-normalized movement", t, func() {
		So(SNM(2, 1), ShouldAlmostEqual, 0.5)
		So(SNM(1, 2), ShouldAlmostEqual, -1.0/3.0)
		So(SNM(5, 5), ShouldEqual, 0)
		So(SNM(0, 0), ShouldEqual, 0)
		So(math.Abs(SNM(1e9, 1e-9)), ShouldBeLessThanOrEqualTo, 1)
	})
}

func TestGrid_Observe(t *testing.T) {
	Convey("Given a grid and one pinned metric", t, func() {
		grid := NewGrid(context.Background())
		source, label, region := pinned()

		observe := func(frame *data.Value, symbol string) *Frame {
			out, err := grid.Observe(frame, symbol)
			So(err, ShouldBeNil)
			return out
		}

		Convey("The first tick of a symbol is warm-up, not a zero SNM", func() {
			frame := observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			So(frame.Regions, ShouldBeNil)
			So(frame.Warming, ShouldEqual, 1)
		})

		Convey("The second tick has SNM but no energy prediction yet", func() {
			observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			frame := observe(tick("BTC/USD", source, label, 2), "BTC/USD")
			So(frame.Regions, ShouldBeNil)
			So(frame.Warming, ShouldEqual, 1)
		})

		Convey("From the third tick a region is lit by |SNM| * surprise", func() {
			observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			observe(tick("BTC/USD", source, label, 2), "BTC/USD")
			frame := observe(tick("BTC/USD", source, label, 8), "BTC/USD")

			So(frame.Warming, ShouldEqual, 0)
			So(frame.Regions, ShouldHaveLength, RegionCount)
			So(frame.Regions[0].Key, ShouldEqual, region)

			movement := SNM(8, 2)
			surprise := math.Abs(SNM(movement*movement, SNM(2, 1)*SNM(2, 1)))
			So(surprise, ShouldBeGreaterThan, 0)
			So(frame.Regions[0].Brightness, ShouldAlmostEqual, math.Abs(movement)*surprise, 1e-12)
		})

		Convey("Unchanged energy is no surprise: nothing lights", func() {
			observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			observe(tick("BTC/USD", source, label, 2), "BTC/USD")
			frame := observe(tick("BTC/USD", source, label, 4), "BTC/USD")
			So(frame.Warming, ShouldEqual, 0)
			So(frame.Regions[0].Brightness, ShouldEqual, 0)
		})

		Convey("A gap restarts warm-up for the key", func() {
			observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			observe(tick("BTC/USD", source, label, 2), "BTC/USD")
			observe(data.NewValue(data.TickOrigin, data.TickRole, data.TickScope), "BTC/USD")
			frame := observe(tick("BTC/USD", source, label, 4), "BTC/USD")
			So(frame.Regions, ShouldBeNil)
			So(frame.Warming, ShouldEqual, 1)
		})

		Convey("Symbols keep separate priors", func() {
			observe(tick("BTC/USD", source, label, 1), "BTC/USD")
			observe(tick("BTC/USD", source, label, 2), "BTC/USD")
			frame := observe(tick("ETH/USD", source, label, 4), "ETH/USD")
			So(frame.Regions, ShouldBeNil)
			So(frame.Warming, ShouldEqual, 1)
		})

		Convey("Another symbol's layers are not this symbol's", func() {
			frame := observe(tick("ETH/USD", source, label, 4), "BTC/USD")
			So(frame.Regions, ShouldBeNil)
			So(frame.Warming, ShouldEqual, 0)
		})

		Convey("An unpinned label is an error", func() {
			_, err := grid.Observe(tick("BTC/USD", source, "unpinned_mystery_metric", 1), "BTC/USD")
			So(err, ShouldNotBeNil)
		})

		Convey("An unpinned source is an error", func() {
			_, err := grid.Observe(tick("BTC/USD", "resonance", label, 1), "BTC/USD")
			So(err, ShouldNotBeNil)
		})

		Convey("A signal layer without a source tag is an error", func() {
			frame := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
			data.AppendVector(frame, []float64{1}, label)
			frame.Index[len(frame.Index)-1] = "BTC/USD/" + frame.Index[len(frame.Index)-1]

			_, err := grid.Observe(frame, "BTC/USD")
			So(err, ShouldNotBeNil)
		})
	})
}
