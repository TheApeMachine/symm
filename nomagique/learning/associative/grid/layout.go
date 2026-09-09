package grid

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/blas64"
	"gonum.org/v1/gonum/lapack"
	"gonum.org/v1/gonum/lapack/lapack64"
)

const (
	gridDimensions = 2
	gridDirections = gridDimensions + 1
)

/*
restructure retains the best rank-two approximation of the previous sketch
stacked with the incoming signed activation row. Its three-row Gram matrix
supplies the left singular vectors. Only the discarded direction loses energy;
equal eigenvalues must not erase every retained direction.

This incremental truncated SVD is exact for rank at most two and approximate
otherwise. It is not the batch optimum over discarded history. See Brand (2006)
for sequential thin SVD and truncation:
https://www.merl.com/publications/docs/TR2006-059.pdf
*/
func (grid *Space) restructure(row int) error {
	if len(grid.Columns) == 0 {
		return nil
	}

	copy(grid.basis[gridDimensions], grid.activations[row])

	for left := range gridDirections {
		for right := left; right < gridDirections; right++ {
			product := 0.0

			for column := range grid.Columns {
				product += grid.basis[left][column] * grid.basis[right][column]
			}

			grid.gram[left*gridDirections+right] = product
		}
	}

	matrix := blas64.Symmetric{
		Uplo: blas.Upper, N: gridDirections, Stride: gridDirections, Data: grid.gram[:],
	}

	if len(grid.work) == 0 {
		query := [1]float64{}
		lapack64.Syev(lapack.EVCompute, matrix, grid.eigenvalues[:], query[:], -1)
		grid.work = make([]float64, int(query[0]))
	}

	// Syev overwrites its input, including on failure. Preserve the original
	// upper triangle so an error describes the matrix actually submitted.
	original := grid.gram

	if !lapack64.Syev(lapack.EVCompute, matrix, grid.eigenvalues[:], grid.work, len(grid.work)) {
		return grid.layoutFailure(row, original)
	}

	// A Gram matrix is positive semidefinite; a negative eigenvalue from
	// roundoff cannot represent discarded energy.
	discarded := math.Max(0, grid.eigenvalues[0])
	grid.discarded += discarded

	for dimension := range gridDimensions {
		direction := gridDirections - 1 - dimension

		for column := range grid.Columns {
			projection := 0.0

			for component := range gridDirections {
				projection += grid.gram[component*gridDirections+direction] * grid.basis[component][column]
			}

			grid.next[dimension][column] = projection
		}
	}

	for dimension := range gridDimensions {
		grid.basis[dimension], grid.next[dimension] = grid.next[dimension], grid.basis[dimension]
	}

	return nil
}

// layoutFailure captures numerical provenance only after the solver fails.
// No value is replaced or skipped; the original failure remains fatal.
func (grid *Space) layoutFailure(row int, original [gridDirections * gridDirections]float64) error {
	details := []string{fmt.Sprintf(
		"grid: co-activation eigendecomposition did not converge; context=%q committed_version=%d gram_upper=%v",
		grid.Rows[row], grid.Version,
		[6]float64{original[0], original[1], original[2], original[4], original[5], original[8]},
	)}

	for column, identity := range grid.Columns {
		if !grid.Present[row][column] {
			continue
		}
		detail := fmt.Sprintf("source=%q metric=%q raw=%g activation=%g quality=%g",
			identity[0], identity[1], grid.Values[row][column],
			grid.activations[row][column], grid.qualities[row][column],
		)

		if baseline := grid.baselines[row][column]; baseline != nil {
			detail += fmt.Sprintf(" baseline=%+v", baseline.Reading)
		}
		details = append(details, detail)
	}
	return errnie.Error(errnie.Err(errnie.Internal, strings.Join(details, "; "), nil))
}
