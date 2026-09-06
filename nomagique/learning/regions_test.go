package learning

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestGridRegions(t *testing.T) {
	Convey("Given nine quantities in a three-by-three plane", t, func() {
		grid := NewGrid()
		measurement := data.NewMeasurement[float64]("", "first", "source", time.Time{}, time.Time{})

		for index := range 9 {
			measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: float64(index)})
		}

		So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)

		for index := range 9 {
			*grid.Coordinates[index] = [2]float64{float64(index % 3), float64(index / 3)}
			grid.qualities[0][index] = 0.5
		}

		Convey("Uphill activity joins its peak and Otsu selects the stronger basin", func() {
			copy(grid.activations[0], []float64{3, 2, 0, 2, 1, 0, 0, 0, 2})
			regions, version, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(version, ShouldEqual, 1)
			So(regions, ShouldHaveLength, 1)
			So(regions[0], ShouldResemble, Region{ID: 1, Condition: ConditionToken(1, 0, 3), Change: 3, Strength: 18, Authority: 0.5, Members: 4})
		})

		Convey("Equal connected peaks form one deterministic plateau", func() {
			copy(grid.activations[0], []float64{2, -2, 0, 0, 0, 0, 0, 0, 0})
			regions, _, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(regions, ShouldResemble, []Region{{ID: 1, Condition: ConditionToken(1, 0, 2), Change: 2, Strength: 8, Authority: 0.5, Members: 2}})
		})

		Convey("Equally strong separated peaks remain separate", func() {
			grid.activations[0][0], grid.activations[0][8] = 2, -2
			regions, _, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(regions, ShouldHaveLength, 2)
			So(regions[0].ID, ShouldEqual, 1)
			So(regions[1].ID, ShouldEqual, 9)
		})

		Convey("Absent quantities contribute no activity even with retained values", func() {
			grid.activations[0][0] = 5
			grid.Present[0][0] = false
			regions, _, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(regions, ShouldBeEmpty)
		})

		Convey("A different context cannot advance this context's identity", func() {
			measurement.Label = "second"
			So(grid.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			_, version, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(version, ShouldEqual, 1)
			_, version, err = grid.Regions("second")
			So(err, ShouldBeNil)
			So(version, ShouldEqual, 2)
		})

		Convey("An unknown context is an explicit error", func() {
			_, _, err := grid.Regions("absent")
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkGridRegions(b *testing.B) {
	grid := NewGrid()
	measurement := data.NewMeasurement[float64]("", "first", "source", time.Time{}, time.Time{})

	// 404 columns matches the current signal-width workload used by Grid.Step.
	for index := range 404 {
		measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: float64(index)})
	}

	if err := grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
		b.Fatal(err)
	}

	for index := range grid.activations[0] {
		grid.activations[0][index] = float64(index % 7)
		grid.qualities[0][index] = 0.5
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := grid.Regions("first"); err != nil {
			b.Fatal(err)
		}
	}
}
