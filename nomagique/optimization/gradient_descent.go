package optimization

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/optimize"
	"gonum.org/v1/gonum/mat"
)

/*
GradientDescentServer minimizes quadratic objective function using first-order gradient descent.
*/
type GradientDescentServer struct {
	*runtime.System
	x []float64
	fVal float64
	iterations int32
}

func NewGradientDescent(ctx context.Context) *GradientDescentServer {
	server := &GradientDescentServer{
		System: runtime.NewSystem(ctx, "optimization.gradient_descent"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and optimization problem definition.
*/
func (server *GradientDescentServer) Write(ctx context.Context, call GradientDescent_write) error {
	args := call.Args()
	dimVal := int(args.Dim())
	initList, err := args.InitX()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read initX", err))
	}

	matList, err := args.MatrixA()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read matrixA", err))
	}

	vecList, err := args.VectorB()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read vectorB", err))
	}

	if dimVal <= 0 {
		dimVal = initList.Len()
	}

	if dimVal <= 0 {
		dimVal = 2
	}

	initX := make([]float64, dimVal)
	for index := 0; index < dimVal && index < initList.Len(); index++ {
		initX[index] = initList.At(index)
	}

	matA := make([]float64, dimVal*dimVal)
	for index := 0; index < dimVal*dimVal && index < matList.Len(); index++ {
		matA[index] = matList.At(index)
	}

	vecB := make([]float64, dimVal)
	for index := 0; index < dimVal && index < vecList.Len(); index++ {
		vecB[index] = vecList.At(index)
	}

	problem := optimize.Problem{
		Func: func(point []float64) float64 {
			val := 0.0
			for rowIdx := 0; rowIdx < dimVal; rowIdx++ {
				arVal := 0.0
				for colIdx := 0; colIdx < dimVal; colIdx++ {
					arVal += matA[rowIdx*dimVal+colIdx] * point[colIdx]
				}
				val += 0.5*point[rowIdx]*arVal - vecB[rowIdx]*point[rowIdx]
			}
			return val
		},
		Grad: func(grad, point []float64) {
			for rowIdx := 0; rowIdx < dimVal; rowIdx++ {
				grad[rowIdx] = -vecB[rowIdx]
				for colIdx := 0; colIdx < dimVal; colIdx++ {
					grad[rowIdx] += matA[rowIdx*dimVal+colIdx] * point[colIdx]
				}
			}
		},
		Hess: func(hess *mat.SymDense, point []float64) {
			for rowIdx := 0; rowIdx < dimVal; rowIdx++ {
				for colIdx := rowIdx; colIdx < dimVal; colIdx++ {
					hess.SetSym(rowIdx, colIdx, matA[rowIdx*dimVal+colIdx])
				}
			}
		},
	}

	res, err := optimize.Minimize(problem, initX, nil, &optimize.GradientDescent{})

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "optimization failed", err))
	}

	server.x = res.X
	server.fVal = res.F
	server.iterations = int32(res.Stats.MajorIterations)
	return nil
}

/*
Done returns calculated optimization results.
*/
func (server *GradientDescentServer) Done(ctx context.Context, call GradientDescent_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[optimization.gradient_descent.Done] failed to allocate results", err))
	}

	listX, err := results.NewX(int32(len(server.x)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to allocate x list", err))
	}

	for index, item := range server.x {
		listX.Set(index, item)
	}
	results.SetFVal(server.fVal)
	results.SetIterations(server.iterations)
	return nil
}
