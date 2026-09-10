package grid

import (
	"slices"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

// regionFixture calibrates two independent pairs. The first is inverse and
// the second direct; both are sympathetic communities rather than hot cells.
func regionFixture(t testing.TB) *Space {
	t.Helper()
	grid := NewSpaceWithWindow(8)
	measurement := data.NewMeasurement[float64]("", "first", "source", time.Time{}, time.Time{})

	for column := range 4 {
		grid.Column("source", strconv.Itoa(column))
		measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(column), Raw: float64(column)})
	}

	if err := grid.Step([]*data.Measurement[float64]{measurement}); err != nil {
		t.Fatal(err)
	}
	copy(grid.weights, []float64{1, 1, 1, 1})
	copy(grid.qualities[0], []float64{0.5, 0.5, 0.5, 0.5})

	for range 2 {
		seed(grid, []float64{1, -1, 1, 1}, []float64{-1, 1, 1, 1},
			[]float64{1, -1, -1, -1}, []float64{-1, 1, -1, -1})
	}

	if !grid.calibrate() {
		t.Fatal("fixture has no calibrated pair")
	}
	grid.regions.form(grid)
	grid.Formed = true
	return grid
}

func TestSpaceRegions(t *testing.T) {
	Convey("Two fixed sympathetic communities receive changing activation", t, func() {
		grid := regionFixture(t)
		membership := slices.Clone(grid.regions.membership)
		So(membership, ShouldResemble, []int{0, 0, 1, 1})
		copy(grid.activations[0], []float64{2, -2, 1, 1})
		regions, version, err := grid.Regions("first")
		So(err, ShouldBeNil)
		So(version, ShouldEqual, 1)
		So(regions, ShouldResemble, []Region{{
			ID: 1, Condition: ConditionToken(1, 0, 2), Change: 2,
			Strength: 8, Authority: 0.5, Members: 2,
		}})

		Convey("The activation split cannot delete the quieter community", func() {
			copy(grid.activations[0], []float64{1, -1, 3, 3})
			regions, _, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(regions, ShouldHaveLength, 1)
			So(regions[0].ID, ShouldEqual, 3)
			So(regions[0].Members, ShouldEqual, 2)
			So(grid.regions.membership, ShouldResemble, membership)
		})

		Convey("Quiet and missing observations retain completed formation", func() {
			clear(grid.activations[0])
			impulse, err := grid.Impulse("first", time.Now(), time.Time{})
			So(err, ShouldBeNil)
			So(impulse.Ready, ShouldBeTrue)
			So(impulse.Regions, ShouldBeEmpty)
			grid.activations[0][0] = 5
			grid.Present[0][0] = false
			regions, _, err := grid.Regions("first")
			So(err, ShouldBeNil)
			So(regions, ShouldBeEmpty)
			So(grid.regions.membership, ShouldResemble, membership)
			So(grid.Formed, ShouldBeTrue)
		})

		Convey("An unknown context remains an explicit error", func() {
			_, _, err := grid.Regions("absent")
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSpaceRegions(b *testing.B) {
	grid, measurement, _ := formationFixture(b, 404)
	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := grid.Regions(measurement.Label); err != nil {
			b.Fatal(err)
		}
	}
}
