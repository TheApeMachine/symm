package grid

import (
	"math"
	"slices"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
)

/*
seed writes complete bins straight into the window, so a test states the
relationships it means rather than arriving at them through the producers.
Each row is one bin and each entry one quantity's movement in it.
*/
func seed(grid *Space, bins ...[]float64) {
	for _, bin := range bins {
		for column, movement := range bin {
			if math.IsNaN(movement) {
				continue // This quantity was not observed in this bin.
			}
			grid.window.observe(column, movement)
		}
		grid.window.close()
	}
}

/* absent marks a quantity as unobserved in a bin. */
var absent = math.NaN()

func TestSpaceRelax(t *testing.T) {
	Convey("Given two quantities and evidence in a 9:1 ratio", t, func() {
		grid := NewSpace().(*Space)
		grid.column("source", "first")
		grid.column("source", "second")
		grid.version = 1
		grid.present = [][]bool{{true, true}}
		copy(grid.weights, []float64{9, 1})
		*grid.coordinates[1] = [2]float64{2, 0}

		Convey("consistent inverses attract with stronger evidence resisting movement", func() {
			for range 8 {
				seed(grid, []float64{1, -1}, []float64{-1, 1})
			}
			So(grid.calibrate(), ShouldBeTrue)
			grid.relax()
			So(grid.coordinates[0][0], ShouldAlmostEqual, 0.2)
			*grid.coordinates[0] = [2]float64{}
			grid.relax()
			So(grid.coordinates[1][0], ShouldAlmostEqual, 0.2)
			// Starting from the same geometry, movement is 0.2 versus 1.8.
			So(2-grid.coordinates[1][0], ShouldBeGreaterThan, 0.2)
		})

		/*
			Each quantity is standardized against its own behaviour before the
			channels are read, so two quantities that move proportionally are
			the same behaviour reported in different units and are placed
			together. Separation comes from disagreement, not from scale.
		*/
		Convey("a pure difference of scale is not a difference of behaviour", func() {
			for range 8 {
				seed(grid, []float64{1, 40}, []float64{-1, -40})
			}
			reading := grid.window.measure(0, 1)
			So(reading.stable(), ShouldBeTrue)
			So(grid.separation(reading), ShouldAlmostEqual, 0)
		})

		Convey("inconsistent movement repels even from coincident coordinates", func() {
			*grid.coordinates[1] = [2]float64{}
			// The pair agrees as often as it disagrees, so their relationship
			// averages out of noise rather than out of any relationship.
			for range 4 {
				seed(grid,
					[]float64{1, 1}, []float64{1, -1},
					[]float64{-1, 1}, []float64{-1, -1},
				)
			}
			reading := grid.window.measure(0, 1)
			So(reading.stable(), ShouldBeFalse)
			So(grid.separation(reading), ShouldBeGreaterThanOrEqualTo, 1)
			So(grid.calibrate(), ShouldBeTrue)
			grid.cursor = 0
			grid.relax()
			So(math.Hypot(grid.coordinates[1][0], grid.coordinates[1][1]),
				ShouldBeGreaterThan, 0)
		})

		Convey("a pair with no shared bin exerts no force in either direction", func() {
			for range 8 {
				seed(grid, []float64{1, absent}, []float64{-1, absent})
			}
			for range 8 {
				seed(grid, []float64{absent, 1}, []float64{absent, -1})
			}
			So(grid.window.measure(0, 1).shared, ShouldEqual, 0)
			So(grid.calibrate(), ShouldBeFalse)
			So(*grid.coordinates[0], ShouldResemble, [2]float64{})
			So(*grid.coordinates[1], ShouldResemble, [2]float64{2, 0})
		})

		Convey("a point without evidence does not invent a force", func() {
			for range 8 {
				seed(grid, []float64{1, -1}, []float64{-1, 1})
			}
			grid.weights[0] = 0
			So(grid.calibrate(), ShouldBeFalse)
			So(*grid.coordinates[0], ShouldResemble, [2]float64{})
			So(*grid.coordinates[1], ShouldResemble, [2]float64{2, 0})
		})

		Convey("calibrated relationships survive a later absent reading", func() {
			for range 8 {
				seed(grid, []float64{1, -1}, []float64{-1, 1})
			}
			So(grid.calibrate(), ShouldBeTrue)
			grid.present[0][0] = false
			grid.relax()
			So(grid.cursor, ShouldEqual, 0)
			So(grid.coordinates[0][0], ShouldAlmostEqual, 0.2)
			So(*grid.coordinates[1], ShouldResemble, [2]float64{2, 0})
		})
	})

	/*
		The update is a majorization of weighted distance stress, so for fixed
		relationships it can never increase that stress. This is the property
		that makes a streaming layout trustworthy: it cannot wander away from
		its own objective between observations.
	*/
	Convey("Given three quantities with fixed relationships", t, func() {
		grid := NewSpace().(*Space)

		for column := range 3 {
			grid.column("source", strconv.Itoa(column))
		}

		grid.version = 1
		grid.present = [][]bool{{true, true, true}}
		copy(grid.weights, []float64{1, 1, 1})

		// The first two move together, the third against both about half the
		// time, so the three pairs ask for three different separations.
		for range 4 {
			seed(grid,
				[]float64{1, 1, 1}, []float64{-1, -1, 1},
				[]float64{1, 1, -1}, []float64{-1, -1, -1},
			)
		}
		targets := [3][3]float64{}

		for left := range grid.columns {
			for right := range grid.columns {
				if left != right {
					targets[left][right] = grid.separation(grid.window.measure(left, right))
				}
			}
		}
		pull := [3][3]float64{}

		for left := range grid.columns {
			for right := range grid.columns {
				if left != right {
					pull[left][right] = grid.window.measure(left, right).strength()
				}
			}
		}
		stress := func() float64 {
			total := 0.0

			for left := range grid.columns {
				for right := left + 1; right < len(grid.columns); right++ {
					separation := math.Hypot(
						grid.coordinates[left][0]-grid.coordinates[right][0],
						grid.coordinates[left][1]-grid.coordinates[right][1],
					)
					residual := separation - targets[left][right]
					total += pull[left][right] * residual * residual
				}
			}

			return total
		}

		So(grid.calibrate(), ShouldBeTrue)
		previous := stress()
		So(previous, ShouldBeGreaterThan, 0)

		for range 64 {
			for range grid.columns {
				grid.relax()
				next := stress()
				So(next, ShouldBeLessThanOrEqualTo, previous+1e-12)
				previous = next
			}
		}
	})
}

func TestSpaceForm(t *testing.T) {
	Convey("Conflicting pair distances settle when represented stress cannot improve", t, func() {
		grid := NewSpace().(*Space)

		for column := range 32 {
			grid.column("source", strconv.Itoa(column))
			grid.weights[column] = float64(column + 1)
		}

		// Frequencies and phase offsets deliberately ask a two-dimensional
		// layout to satisfy incompatible pair distances, unlike an exact
		// two-cohort fixture. These are solver inputs, not market beliefs.
		for bin := range grid.window.capacity {
			values := make([]float64, len(grid.columns))

			for column := range values {
				values[column] = math.Sin(float64((bin+1)*(column+1))) +
					math.Cos(float64(bin+column))
			}
			seed(grid, values)
		}

		for attempts := 0; attempts < 100000 && !grid.formed; attempts++ {
			grid.form()
		}
		So(grid.formed, ShouldBeTrue)

		for column := range grid.columns {
			position := *grid.coordinates[column]
			grid.relax()
			So(*grid.coordinates[column], ShouldResemble, position)
		}
	})

	Convey("A complete calibration settles once and subsequent activity cannot restart it", t, func() {
		grid := NewSpace(8).(*Space)
		measurement := data.NewMeasurement[float64]("source", nil)
		measurement.Label, measurement.At, measurement.From = "market", time.Time{}, time.Time{}

		for column := range 4 {
			grid.column("source", strconv.Itoa(column))
		}
		tape := market.NewOpportunityTape("market", time.Unix(1, 0), 12)
		steps := 0

		// This bounds a test failure, not production formation. Production has
		// no iteration cutoff and reports ready only after an unchanged sweep.
		for steps < 8192 && !grid.formed {
			event := tape.Steps[steps%len(tape.Steps)]
			measurement.At = event.EventTime
			measurement.Maturity = 1
			measurement.SNR, measurement.SNRDefined = 100, true
			values := []float64{event.Context, -event.Context, event.ExecutableBid, event.ExecutableBid}

			for column, value := range values {
				measurement.Metrics[strconv.Itoa(column)] = data.Metric[float64]{Label: strconv.Itoa(column), Raw: value}
			}
			So(grid.step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			steps++
		}
		So(grid.formed, ShouldBeTrue)
		So(steps, ShouldBeGreaterThan, grid.window.capacity)
		coordinates := make([][2]float64, len(grid.columns))

		for column := range coordinates {
			coordinates[column] = *grid.coordinates[column]
		}
		membership := slices.Clone(grid.regions.membership)

		for _, event := range tape.Steps {
			measurement.Metrics = map[string]data.Metric[float64]{
				"0": {Label: "0", Raw: event.ExecutableBid},
			}
			So(grid.step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
			impulse, err := grid.impulse("market", event.EventTime, time.Unix(1, 0))
			So(err, ShouldBeNil)
			So(impulse.Ready, ShouldBeTrue)
			So(grid.regions.membership, ShouldResemble, membership)

			for column, expected := range coordinates {
				So(*grid.coordinates[column], ShouldResemble, expected)
			}
		}

		Convey("Adding a new quantity is an explicit schema change", func() {
			countBefore := grid.window.count
			grid.column("source", "new quantity")
			So(grid.formed, ShouldBeFalse)
			So(grid.graph, ShouldBeNil)
			So(grid.window.count, ShouldEqual, countBefore)
		})
	})
}

// formationFixture supplies four repeating sign regimes across the production
// calibration span. Pairs within a cohort share movement; the cohorts do not.
func formationFixture(t testing.TB, columns int) (*Space, *data.Measurement[float64], int) {
	t.Helper()
	grid := NewSpace().(*Space)
	measurement := data.NewMeasurement[float64]("source", nil)
	measurement.Label, measurement.At, measurement.From = "market", time.Time{}, time.Time{}
	measurement.Maturity = 1
	measurement.SNR, measurement.SNRDefined = 100, true

	for column := range columns {
		grid.column("source", strconv.Itoa(column))
	}
	steps := 0

	for !grid.formed {
		// A test-only execution budget catches a broken convergence loop.
		if steps == 100000 {
			t.Fatalf("formation did not settle: columns=%d cursor=%d", columns, grid.cursor)
		}

		for column := range columns {
			value := float64((steps/(1+column%2))%2)*2 - 1
			measurement.Metrics[strconv.Itoa(column)] = data.Metric[float64]{Label: strconv.Itoa(column), Raw: value}
		}

		if err := grid.step([]*data.Measurement[float64]{measurement}); err != nil {
			t.Fatal(err)
		}
		steps++
	}
	return grid, measurement, steps
}

func BenchmarkSpaceForm(b *testing.B) {
	for _, columns := range []int{32, 404} {
		b.Run(strconv.Itoa(columns), func(b *testing.B) {
			b.ReportAllocs()
			var steps int

			for b.Loop() {
				_, _, steps = formationFixture(b, columns)
			}
			b.ReportMetric(float64(steps), "observations/form")
		})
	}
}
