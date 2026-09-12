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
	grid := NewSpace(8).(*Space)
	measurement := data.NewMeasurement[float64]("source", nil)
	measurement.Label, measurement.At, measurement.From = "first", time.Time{}, time.Time{}

	for column := range 4 {
		grid.column("source", strconv.Itoa(column))
		measurement.Metrics[strconv.Itoa(column)] = data.Metric[float64]{Label: strconv.Itoa(column), Raw: float64(column)}
	}

	if err := grid.step([]*data.Measurement[float64]{measurement}); err != nil {
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
	grid.formed = true
	return grid
}

func TestSpaceRegions(t *testing.T) {
	Convey("Two fixed sympathetic communities receive changing activation", t, func() {
		grid := regionFixture(t)
		membership := slices.Clone(grid.regions.membership)
		So(membership, ShouldResemble, []int{0, 0, 1, 1})
		copy(grid.activations[0], []float64{2, -2, 1, 1})
		regions, version, err := grid.regionsOf("first")
		So(err, ShouldBeNil)
		So(version, ShouldEqual, 1)
		So(regions, ShouldResemble, []Region{{
			ID: 1, Condition: mustConditionToken(t, 1, 0, 2), Change: 2,
			Strength: 8, Authority: 0.5, Members: 2,
		}})

		Convey("The activation split cannot delete the quieter community", func() {
			copy(grid.activations[0], []float64{1, -1, 3, 3})
			regions, _, err := grid.regionsOf("first")
			So(err, ShouldBeNil)
			So(regions, ShouldHaveLength, 1)
			So(regions[0].ID, ShouldEqual, 3)
			So(regions[0].Members, ShouldEqual, 2)
			So(grid.regions.membership, ShouldResemble, membership)
		})

		Convey("Quiet and missing observations retain completed formation", func() {
			clear(grid.activations[0])
			impulse, err := grid.impulse("first", time.Now(), time.Time{})
			So(err, ShouldBeNil)
			So(impulse.Ready, ShouldBeTrue)
			So(impulse.Regions, ShouldBeEmpty)
			grid.activations[0][0] = 5
			grid.present[0][0] = false
			regions, _, err := grid.regionsOf("first")
			So(err, ShouldBeNil)
			So(regions, ShouldBeEmpty)
			So(grid.regions.membership, ShouldResemble, membership)
			So(grid.formed, ShouldBeTrue)
		})

		Convey("An unknown context remains an explicit error", func() {
			_, _, err := grid.regionsOf("absent")
			So(err, ShouldNotBeNil)
		})

		Convey("Community density normalization resists absorption by larger weak clusters", func() {
			testGrid := NewSpace(8).(*Space)
			testMeasurement := data.NewMeasurement[float64]("source", nil)
			testMeasurement.Label, testMeasurement.At, testMeasurement.From = "test", time.Time{}, time.Time{}

			for column := range 6 {
				testGrid.column("source", strconv.Itoa(column))
				testMeasurement.Metrics[strconv.Itoa(column)] = data.Metric[float64]{Label: strconv.Itoa(column), Raw: float64(column)}
			}

			err := testGrid.step([]*data.Measurement[float64]{testMeasurement})
			So(err, ShouldBeNil)
			copy(testGrid.weights, []float64{1, 1, 1, 1, 1, 1})
			copy(testGrid.qualities[0], []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5})

			testGrid.graph = make([][]affinity, 6)

			for column := range testGrid.graph {
				testGrid.graph[column] = make([]affinity, 6)
			}

			// Cluster A: nodes 0, 1, 2, 3 have weak mutual links of 0.2
			weakReading := affinity{directional: 0.2, consistency: 0.6, magnitude: 0.2, shared: 8}

			for left := 0; left < 4; left++ {
				for right := left + 1; right < 4; right++ {
					testGrid.graph[left][right] = weakReading
					testGrid.graph[right][left] = weakReading
				}
			}

			// Node 4 and 5 have a strong mutual link of 0.9
			strongReading := affinity{directional: 0.9, consistency: 0.9, magnitude: 0.9, shared: 8}
			testGrid.graph[4][5] = strongReading
			testGrid.graph[5][4] = strongReading

			// Node 4 also has weak links of 0.2 to all 4 nodes in Cluster A
			for peer := 0; peer < 4; peer++ {
				testGrid.graph[4][peer] = weakReading
				testGrid.graph[peer][4] = weakReading
			}

			testGrid.regions.form(testGrid)

			// Node 4 must stay in the same community as node 5 because mean affinity (0.9) > weak affinity (0.2)
			So(testGrid.regions.membership[4], ShouldEqual, testGrid.regions.membership[5])
			So(testGrid.regions.membership[4], ShouldNotEqual, testGrid.regions.membership[0])
		})
	})
}

func BenchmarkSpaceRegions(b *testing.B) {
	grid, measurement, _ := formationFixture(b, 404)
	b.ReportAllocs()

	for b.Loop() {
		if _, _, err := grid.regionsOf(measurement.Label); err != nil {
			b.Fatal(err)
		}
	}
}

/* mustConditionToken builds one condition token, failing the test if the identity does not fit. */
func mustConditionToken(t testing.TB, quantity uint64, level, change float64) uint64 {
	t.Helper()

	token, err := conditionToken(quantity, level, change)

	if err != nil {
		t.Fatal(err)
	}

	return token
}
