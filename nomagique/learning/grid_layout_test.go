package learning

import (
	"math"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
)

func TestGridRestructure(t *testing.T) {
	Convey("Given signed activation rows with known second moments", t, func() {
		cases := []struct {
			name  string
			rows  [][]float64
			exact bool
		}{
			{"two independent profiles and their copies", [][]float64{
				{1, 1, -1, 0}, {-1, -1, 1, 0},
				{0, 0, 0, 1}, {0, 0, 0, -1},
			}, true},
			{"three equally strong independent profiles", [][]float64{
				{1, 0, 0}, {0, 1, 0}, {0, 0, 1},
			}, false},
			{"changing profiles exceeding the output dimension", [][]float64{
				{1, -1, 2, 0}, {0, 2, -1, 1}, {2, 0, 1, -1},
				{-1, 1, 0, 2}, {1, 2, -1, 0}, {0, -1, 2, 1},
			}, false},
		}

		for _, example := range cases {
			Convey(example.name, func() {
				grid := NewGrid()
				width := len(example.rows[0])

				for column := range width {
					grid.column("source", strconv.Itoa(column))
				}

				grid.activations = [][]float64{make([]float64, width)}
				expected := mat.NewSymDense(width, nil)

				for _, values := range example.rows {
					copy(grid.activations[0], values)
					So(grid.restructure(0), ShouldBeNil)
					grid.Version++

					for left := range width {
						for right := left; right < width; right++ {
							expected.SetSym(left, right,
								expected.At(left, right)+values[left]*values[right])
						}
					}
				}

				loss := mat.NewSymDense(width, nil)
				retainedEnergy := 0.0

				for left := range width {
					for right := left; right < width; right++ {
						retained := grid.basis[0][left]*grid.basis[0][right] +
							grid.basis[1][left]*grid.basis[1][right]
						loss.SetSym(left, right, expected.At(left, right)-retained)

						if left == right {
							retainedEnergy += retained
						}
					}
				}

				So(retainedEnergy, ShouldBeGreaterThan, 0)
				decomposition := mat.EigenSym{}
				So(decomposition.Factorize(loss, false), ShouldBeTrue)
				eigenvalues := decomposition.Values(nil)
				// Roundoff allowance for the small, explicitly bounded fixtures.
				const tolerance = 1e-9
				So(eigenvalues[0], ShouldBeGreaterThanOrEqualTo, -tolerance)
				So(eigenvalues[width-1], ShouldBeLessThanOrEqualTo,
					grid.CovarianceError()*float64(grid.Version)+tolerance)

				if example.exact {
					So(math.Abs(eigenvalues[width-1]), ShouldBeLessThan, tolerance)
					So(grid.CovarianceError(), ShouldAlmostEqual, 0)
				}
			})
		}
	})
}
