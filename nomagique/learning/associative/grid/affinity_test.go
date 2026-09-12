package grid

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/* publish sends one source's single metric as its own update. */
func publish(t testing.TB, grid *Space, source string, value float64) {
	t.Helper()
	measurement := data.NewMeasurement[float64](source, nil)
	measurement.Label, measurement.At, measurement.From = "context", time.Time{}, time.Time{}
	measurement.Metrics["m"] = data.Metric[float64]{Label: "m", Raw: value}
	So(grid.step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
}

/*
Producers are driven by different raw frames, so a trade-driven quantity and a
book-driven quantity are published on separate updates and never share one.
Reading affinity from single updates therefore measured the transport rather
than the market: two quantities moving identically were reported as unrelated
purely because they never arrived together.
*/
func TestAffinityAcrossSeparateUpdates(t *testing.T) {
	Convey("Quantities that never arrive together are still comparable", t, func() {
		swing := func(tick int) float64 {
			if tick%2 == 1 {
				return 1
			}

			return -1
		}

		Convey("Identical movement reads as identical however it arrives", func() {
			together, apart := NewSpace().(*Space), NewSpace().(*Space)

			for tick := range 120 {
				value := swing(tick)
				pair := data.NewMeasurement[float64]("alpha", nil)
				pair.Label, pair.At, pair.From = "context", time.Time{}, time.Time{}
				pair.Metrics["m"] = data.Metric[float64]{Label: "m", Raw: value}
				other := data.NewMeasurement[float64]("beta", nil)
				other.Label, other.At, other.From = "context", time.Time{}, time.Time{}
				other.Metrics["m"] = data.Metric[float64]{Label: "m", Raw: value}
				So(together.step([]*data.Measurement[float64]{pair, other}), ShouldBeNil)

				// The same two quantities, each on its own update.
				publish(t, apart, "alpha", value)
				publish(t, apart, "beta", value)
			}

			for _, grid := range []*Space{together, apart} {
				left := grid.columnIndex[[2]string{"alpha", "m"}]
				right := grid.columnIndex[[2]string{"beta", "m"}]
				reading := grid.window.measure(left, right)
				So(reading.shared, ShouldBeGreaterThan, 1)
				So(reading.directional, ShouldAlmostEqual, 1, 1e-9)
				So(reading.consistency, ShouldAlmostEqual, 1, 1e-9)
				So(reading.stable(), ShouldBeTrue)
			}
		})

		Convey("Opposite movement reads as inverse, which is still a relationship", func() {
			grid := NewSpace().(*Space)

			for tick := range 120 {
				publish(t, grid, "alpha", swing(tick))
				publish(t, grid, "beta", -swing(tick))
			}
			reading := grid.window.measure(
				grid.columnIndex[[2]string{"alpha", "m"}],
				grid.columnIndex[[2]string{"beta", "m"}],
			)
			So(reading.directional, ShouldAlmostEqual, -1, 1e-9)
			So(reading.consistency, ShouldAlmostEqual, -1, 1e-9)
			So(reading.orientation(), ShouldEqual, -1)

			// A consistent inverse attracts: it is a relationship, not noise.
			So(reading.stable(), ShouldBeTrue)
			So(grid.separation(reading), ShouldBeLessThan, 1)
		})

		Convey("A pair that never shared a bin supports no reading at all", func() {
			grid := NewSpace().(*Space)

			// beta only ever publishes after the window has moved past alpha.
			for range 120 {
				publish(t, grid, "alpha", 1)
				publish(t, grid, "alpha", -1)
			}
			for range 200 {
				publish(t, grid, "beta", 1)
				publish(t, grid, "beta", -1)
			}
			reading := grid.window.measure(
				grid.columnIndex[[2]string{"alpha", "m"}],
				grid.columnIndex[[2]string{"beta", "m"}],
			)
			So(reading.shared, ShouldEqual, 0)
			So(reading.strength(), ShouldEqual, 0)
		})
	})
}

func TestAffinityChannelsSeparateConcerns(t *testing.T) {
	Convey("The channels answer different questions about one pair", t, func() {
		retained := newWindow(8)
		retained.columns(2)

		Convey("Agreeing in direction while disagreeing in size", func() {
			// Same sign every bin; one quantity moves in far larger units.
			for _, pair := range [][2]float64{{1, 100}, {2, 5}, {1, 90}, {3, 4}, {1, 80}, {2, 6}} {
				retained.observe(0, pair[0])
				retained.observe(1, pair[1])
				retained.close()
			}
			reading := retained.measure(0, 1)
			So(reading.shared, ShouldEqual, 6)

			// Consistency is measured on standardized signs, so a pair that
			// always rises together can still disagree about how much.
			So(reading.magnitude, ShouldBeGreaterThan, 0)
			So(reading.strength(), ShouldBeGreaterThan, 0)
		})

		Convey("A quantity that never varied supports no co-movement", func() {
			retained := newWindow(8)
			retained.columns(2)

			for index := range 6 {
				retained.observe(0, float64(index%2))
				retained.observe(1, 4)
				retained.close()
			}
			reading := retained.measure(0, 1)
			So(reading.shared, ShouldEqual, 6)
			So(reading.directional, ShouldEqual, 0)
			So(reading.strength(), ShouldEqual, 0)
		})
	})
}
func TestWindowBinsRotation(t *testing.T) {
	Convey("A bin spans one rotation of whatever is publishing", t, func() {
		retained := newWindow(4)
		retained.columns(2)

		// Bootstrap: nothing is required yet, so a repeat is the only rotation
		// evidence available, and it belongs to the bin it opens.
		retained.observe(0, 1)
		retained.observe(1, 7)
		So(retained.covered(), ShouldBeFalse)
		retained.observe(0, 2)
		So(retained.count, ShouldEqual, 1)
		So(retained.slots[0][0], ShouldEqual, 1)
		So(retained.slots[0][1], ShouldEqual, 7)

		/*
			The slow quantity has not spoken again yet. A bin sized to the
			fastest producer would shut here and the two would never share a
			bin, which is exactly the failure being removed.
		*/
		So(retained.covered(), ShouldBeFalse)
		retained.observe(1, 8)
		So(retained.covered(), ShouldBeTrue)
		retained.close()
		So(retained.count, ShouldEqual, 2)
		So(retained.present[1][0], ShouldBeTrue)
		So(retained.present[1][1], ShouldBeTrue)
		So(retained.slots[1][0], ShouldEqual, 2)

		Convey("A producer that goes silent stops holding the bin open", func() {
			for range retained.capacity + 1 {
				retained.observe(0, 1)
				retained.close()
			}
			So(retained.membership[1], ShouldBeFalse)
			retained.observe(0, 1)
			So(retained.covered(), ShouldBeTrue)
		})

		Convey("Retained bins never exceed the declared span", func() {
			for range 20 {
				retained.observe(0, 1)
				retained.observe(1, 1)
				retained.close()
			}
			So(retained.count, ShouldEqual, retained.capacity)
			So(len(retained.slots), ShouldEqual, retained.capacity)
		})
	})
}

/*
BenchmarkSpaceStepFrameTypes is the shape the live grid actually sees: three
producer families driven by different raw frames, each publishing its own
quantities on its own update, across many instruments. It is the arrangement
that made a single-update sketch report unrelated quantities as related and
identical ones as orthogonal, so it is the arrangement the cost is measured in.
*/
func BenchmarkSpaceStepFrameTypes(b *testing.B) {
	grid := NewSpace().(*Space)
	families := []string{"ticker", "trade", "level3"}
	build := func(family, label string, tick int) *data.Measurement[float64] {
		measurement := data.NewMeasurement[float64](family, nil)
		measurement.Label, measurement.At, measurement.From = label, time.Time{}, time.Time{}
		for metric := range 290 {
			value := float64((tick+metric)%7) - 3
			measurement.Metrics[strconv.Itoa(metric)] = data.Metric[float64]{Label: strconv.Itoa(metric), Raw: value}
		}
		return measurement
	}
	labels := make([]string, 64)
	for index := range labels {
		labels[index] = strconv.Itoa(index)
	}
	for tick := range 8 {
		for _, label := range labels {
			for _, family := range families {
				if err := grid.step([]*data.Measurement[float64]{build(family, label, tick)}); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	tick := 0
	for b.Loop() {
		tick++
		family := families[tick%len(families)]
		label := labels[tick%len(labels)]
		if err := grid.step([]*data.Measurement[float64]{build(family, label, tick)}); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(grid.columns)), "columns")
}
