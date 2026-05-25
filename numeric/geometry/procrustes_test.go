package geometry

import (
	"math"
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcrustes(t *testing.T) {
	Convey("Given the Procrustes orthogonal alignment solver", t, func() {
		Convey("When A equals B (identity alignment)", func() {
			nDim := 8
			nSamples := 12

			matA := randomMatrix(nSamples, nDim, 42)
			matB := copyMatrix(matA)

			result, err := Procrustes(matA, matB, nSamples, nDim)

			Convey("It should return R ≈ I with near-zero residual", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)
				So(result.Residual, ShouldBeLessThan, 1e-10)

				for row := 0; row < nDim; row++ {
					for col := 0; col < nDim; col++ {
						expected := 0.0
						if row == col {
							expected = 1.0
						}

						So(result.R[row][col], ShouldAlmostEqual, expected, 1e-8)
					}
				}
			})
		})

		Convey("When B = R·A for a known 90° rotation in the first two dims", func() {
			nDim := 4
			nSamples := 6

			matA := randomMatrix(nSamples, nDim, 99)

			knownR := eye(nDim)
			knownR[0][0] = 0
			knownR[0][1] = -1
			knownR[1][0] = 1
			knownR[1][1] = 0

			matB := make([][]float64, nSamples)
			for sample := 0; sample < nSamples; sample++ {
				matB[sample] = make([]float64, nDim)

				for dim := 0; dim < nDim; dim++ {
					for inner := 0; inner < nDim; inner++ {
						matB[sample][dim] += knownR[dim][inner] * matA[sample][inner]
					}
				}
			}

			result, err := Procrustes(matA, matB, nSamples, nDim)

			Convey("It should recover the known rotation with low residual", func() {
				So(err, ShouldBeNil)
				So(result, ShouldNotBeNil)
				So(result.Residual, ShouldBeLessThan, 1e-8)

				for row := 0; row < nDim; row++ {
					for col := 0; col < nDim; col++ {
						So(result.R[row][col], ShouldAlmostEqual, knownR[row][col], 1e-6)
					}
				}
			})
		})

		Convey("When given degenerate inputs", func() {
			Convey("It should error on zero dimensions", func() {
				_, err := Procrustes(nil, nil, 0, 0)
				So(err, ShouldNotBeNil)
			})

			Convey("It should error on row count mismatch", func() {
				matA := randomMatrix(3, 2, 1)
				matB := randomMatrix(4, 2, 2)
				_, err := Procrustes(matA, matB, 3, 2)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestJacobiSVD(t *testing.T) {
	Convey("Given the Jacobi SVD decomposition", t, func() {
		Convey("When decomposing a known diagonal matrix", func() {
			mat := [][]float64{
				{3, 0, 0},
				{0, 5, 0},
				{0, 0, 2},
			}

			uMat, sigma, vMat, err := JacobiSVD(mat, 3, 3)

			Convey("It should recover the singular values", func() {
				So(err, ShouldBeNil)
				So(len(sigma), ShouldEqual, 3)

				sorted := make([]float64, len(sigma))
				copy(sorted, sigma)

				for idx := range sorted {
					found := false

					for _, expected := range []float64{5, 3, 2} {
						if math.Abs(sorted[idx]-expected) < 1e-8 {
							found = true
							break
						}
					}

					So(found, ShouldBeTrue)
				}

				So(uMat, ShouldNotBeNil)
				So(vMat, ShouldNotBeNil)
			})
		})

		Convey("When decomposing a general matrix", func() {
			mat := [][]float64{
				{1, 2},
				{3, 4},
				{5, 6},
			}

			uMat, sigma, vMat, err := JacobiSVD(mat, 3, 2)

			Convey("It should reconstruct A = U·Σ·Vᵀ", func() {
				So(err, ShouldBeNil)

				for row := 0; row < 3; row++ {
					for col := 0; col < 2; col++ {
						var reconstructed float64

						for inner := 0; inner < 2; inner++ {
							reconstructed += uMat[row][inner] * sigma[inner] * vMat[col][inner]
						}

						So(reconstructed, ShouldAlmostEqual, mat[row][col], 1e-8)
					}
				}
			})
		})

		Convey("When rows < cols", func() {
			Convey("It should return an error", func() {
				mat := [][]float64{{1, 2, 3}}
				_, _, _, err := JacobiSVD(mat, 1, 3)
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkProcrustes512(b *testing.B) {
	nDim := 512
	nSamples := 16

	matA := randomMatrix(nSamples, nDim, 1337)
	matB := randomMatrix(nSamples, nDim, 7331)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = Procrustes(matA, matB, nSamples, nDim)
	}
}

func BenchmarkJacobiSVD512(b *testing.B) {
	nDim := 512
	mat := randomMatrix(nDim, nDim, 2024)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _, _ = JacobiSVD(mat, nDim, nDim)
	}
}

func randomMatrix(rows, cols int, seed int64) [][]float64 {
	rng := rand.New(rand.NewSource(seed))
	mat := make([][]float64, rows)

	for row := range mat {
		mat[row] = make([]float64, cols)

		for col := range mat[row] {
			mat[row][col] = rng.NormFloat64()
		}
	}

	return mat
}

func copyMatrix(src [][]float64) [][]float64 {
	dst := make([][]float64, len(src))

	for row := range src {
		dst[row] = make([]float64, len(src[row]))
		copy(dst[row], src[row])
	}

	return dst
}
