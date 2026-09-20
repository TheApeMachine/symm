package algo

import (
	"context"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

type GaussJordanServer struct {
	DownstreamGaussJordan func(context.Context, [][]float64) error
	Tolerance             float64
}

func (s *GaussJordanServer) Evaluate(ctx context.Context, left [][]float64, right [][]float64) ([][]float64, error) {
	rows := len(left)
	if rows == 0 {
		return nil, nil
	}

	tol := s.Tolerance
	if tol <= 0 {
		tol = 1e-9
	}

	rightCols := 0
	if rows > 0 && len(right) > 0 {
		rightCols = len(right[0])
	}

	a := make([][]float64, rows)
	b := make([][]float64, rows)
	for i := 0; i < rows; i++ {
		a[i] = make([]float64, rows)
		copy(a[i], left[i])

		b[i] = make([]float64, rightCols)
		copy(b[i], right[i])
	}

	for col := 0; col < rows; col++ {
		pivotRow := col
		maxVal := math.Abs(a[col][col])
		for r := col + 1; r < rows; r++ {
			val := math.Abs(a[r][col])
			if val > maxVal {
				maxVal = val
				pivotRow = r
			}
		}
		if maxVal <= tol {
			return nil, nil
		}
		if pivotRow != col {
			a[col], a[pivotRow] = a[pivotRow], a[col]
			b[col], b[pivotRow] = b[pivotRow], b[col]
		}
		pivot := a[col][col]
		for c := col; c < rows; c++ {
			a[col][c] /= pivot
		}
		for c := 0; c < rightCols; c++ {
			b[col][c] /= pivot
		}
		for r := 0; r < rows; r++ {
			if r != col {
				factor := a[r][col]
				for c := col; c < rows; c++ {
					a[r][c] -= factor * a[col][c]
				}
				for c := 0; c < rightCols; c++ {
					b[r][c] -= factor * b[col][c]
				}
			}
		}
	}
	
	if s.DownstreamGaussJordan != nil {
		return b, s.DownstreamGaussJordan(ctx, b)
	}
	
	return b, nil
}

func (s *GaussJordanServer) Write(ctx context.Context, call GaussJordan_write) error {
	left, _ := call.Args().Left()
	right, _ := call.Args().Right()
	rows := left.Len()
	if rows == 0 {
		return nil
	}

	rightCols := 0
	if rows > 0 && right.Len() > 0 {
		right0 := right.At(0)
		vals, _ := right0.Values()
		rightCols = vals.Len()
	}

	a := make([][]float64, rows)
	b := make([][]float64, rows)
	for i := 0; i < rows; i++ {
		leftRow := left.At(i)
		leftVals, _ := leftRow.Values()
		a[i] = make([]float64, rows)
		for j := 0; j < rows; j++ {
			a[i][j] = leftVals.At(j)
		}

		rightRow := right.At(i)
		rightVals, _ := rightRow.Values()
		b[i] = make([]float64, rightCols)
		for j := 0; j < rightCols; j++ {
			b[i][j] = rightVals.At(j)
		}
	}

	_, err := s.Evaluate(ctx, a, b)
	return err
}

func (s *GaussJordanServer) Done(ctx context.Context, call GaussJordan_done) error {
	return nil
}

type GaussJordanNode types.StreamNode[[2][][]float64, [][]float64]

func NewGaussJordan(tolerance types.Float) GaussJordanNode {
	server := &GaussJordanServer{}
	return types.NewStreamNode(server, func(ctx context.Context, in any) error {
		input := in.([2][][]float64)
		server.Tolerance = 1e-9
		if tolerance != nil {
			server.Tolerance = tolerance(input)
		}
		_, err := server.Evaluate(ctx, input[0], input[1])
		return err
	}, func(next func(context.Context, any) error) {
		server.DownstreamGaussJordan = func(ctx context.Context, res [][]float64) error {
			return next(ctx, res)
		}
	})
}
