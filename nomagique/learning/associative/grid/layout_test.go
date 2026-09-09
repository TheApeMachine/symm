package grid

import (
	"math"
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"gonum.org/v1/gonum/mat"
)

func TestSpaceRestructure(t *testing.T) {
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
				grid := NewSpace()
				width := len(example.rows[0])

				for column := range width {
					grid.Column("source", strconv.Itoa(column))
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

func TestSpaceLayoutFailure(t *testing.T) {
	Convey("A solver failure retains the submitted matrix after LAPACK overwrites its workspace", t, func() {
		grid := NewSpace()
		grid.Rows = []string{"BTC/USD"}
		grid.Values = [][]float64{nil}
		grid.Present = [][]bool{nil}
		grid.activations = [][]float64{nil}
		grid.qualities = [][]float64{nil}
		grid.Version = 23
		// A three-dimensional symmetric matrix has six independent entries.
		original := [gridDirections * gridDirections]float64{1, 2, 3, 0, 4, 5, 0, 0, 6}
		grid.gram = [gridDirections * gridDirections]float64{9, 9, 9, 9, 9, 9, 9, 9, 9}
		err := grid.layoutFailure(0, original)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, `context="BTC/USD" committed_version=23`)
		So(err.Error(), ShouldContainSubstring, "gram_upper=[1 2 3 4 5 6]")
		So(grid.Version, ShouldEqual, 23)
	})
}

func BenchmarkSpaceRestructure(b *testing.B) {
	// Match the 404-coordinate workload used by BenchmarkSpaceStep while
	// measuring the sketch update independently of producer/estimator costs.
	grid := NewSpace()

	for column := range 404 {
		grid.Column("source", strconv.Itoa(column))
	}
	grid.activations = [][]float64{make([]float64, len(grid.Columns))}
	b.ReportAllocs()
	step := 0

	for b.Loop() {
		for column := range grid.Columns {
			grid.activations[0][column] = float64((column+step)%3) - 1
		}

		if err := grid.restructure(0); err != nil {
			b.Fatal(err)
		}
		step++
	}
}
