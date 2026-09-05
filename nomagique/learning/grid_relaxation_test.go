package learning

import (
	"math"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGridRelax(t *testing.T) {
	Convey("Given two profiles and evidence in a 9:1 ratio", t, func() {
		grid := NewGrid()
		grid.column("source", "first")
		grid.column("source", "second")
		grid.Version = 1
		grid.Present = [][]bool{{true, true}}
		copy(grid.weights, []float64{9, 1})
		copy(grid.basis[0], []float64{1, -1})
		*grid.Coordinates[1] = [2]float64{2, 0}

		Convey("consistent inverses attract with stronger evidence resisting movement", func() {
			grid.relax(0)
			So(grid.Coordinates[0][0], ShouldAlmostEqual, 0.2)
			*grid.Coordinates[0] = [2]float64{}
			grid.relax(0)
			So(grid.Coordinates[1][0], ShouldAlmostEqual, 0.2)
			// Starting from the same geometry, movement is 0.2 versus 1.8.
			So(2-grid.Coordinates[1][0], ShouldBeGreaterThan, 0.2)
		})

		Convey("different relative magnitudes retain a nonzero separation", func() {
			grid.basis[0][1] = -2
			grid.cursor = 0
			grid.relax(0)
			So(grid.Coordinates[1][0], ShouldAlmostEqual, 1.1)
		})

		Convey("inconsistent movement repels even from coincident coordinates", func() {
			*grid.Coordinates[1] = [2]float64{}
			grid.basis[0][1] = 0
			grid.basis[1][1] = 1
			grid.cursor = 0
			grid.relax(0)
			So(math.Hypot(grid.Coordinates[1][0], grid.Coordinates[1][1]),
				ShouldBeGreaterThan, 0)
		})

		Convey("a point without evidence does not invent a force", func() {
			grid.weights[0] = 0
			grid.relax(0)
			grid.relax(0)
			So(*grid.Coordinates[0], ShouldResemble, [2]float64{})
			So(*grid.Coordinates[1], ShouldResemble, [2]float64{2, 0})
		})

		Convey("an absent reading does not attract or repel a present reading", func() {
			grid.Present[0][0] = false
			grid.relax(0)
			So(grid.cursor, ShouldEqual, 1)
			So(*grid.Coordinates[0], ShouldResemble, [2]float64{})
			So(*grid.Coordinates[1], ShouldResemble, [2]float64{2, 0})
		})
	})

	Convey("Given three fixed profiles whose pair distances form a triangle", t, func() {
		grid := NewGrid()

		for column := range 3 {
			grid.column("source", strconv.Itoa(column))
		}

		grid.Version = 1
		grid.Present = [][]bool{{true, true, true}}
		copy(grid.weights, []float64{1, 1, 1})
		copy(grid.basis[0], []float64{1, 0, 1})
		copy(grid.basis[1], []float64{0, 1, 1})
		// The profiles are (1,0), (0,1), (1,1). Their sign-invariant
		// separations are sqrt(2), 1, 1, all realizable in this plane.
		targets := [3][3]float64{{0, math.Sqrt(2), 1}, {math.Sqrt(2), 0, 1}, {1, 1, 0}}
		stress := func() float64 {
			total := 0.0

			for left := range grid.Columns {
				for right := left + 1; right < len(grid.Columns); right++ {
					separation := math.Hypot(
						grid.Coordinates[left][0]-grid.Coordinates[right][0],
						grid.Coordinates[left][1]-grid.Coordinates[right][1],
					)
					residual := separation - targets[left][right]
					total += residual * residual
				}
			}

			return total
		}

		previous := stress()
		So(previous, ShouldAlmostEqual, 4)
		// This small fixed triangle settles within 64 complete sweeps.
		// The production path does one coordinate per incoming update.
		for range 64 {
			for range grid.Columns {
				grid.relax(0)
				next := stress()
				So(next, ShouldBeLessThanOrEqualTo, previous+1e-12)
				previous = next
			}
		}

		So(previous, ShouldBeLessThan, 1e-9)
	})
}
