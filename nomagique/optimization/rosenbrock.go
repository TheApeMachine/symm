package optimization

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/optimize"
)

/*
RosenbrockServer minimizes non-convex Rosenbrock banana benchmark function using BFGS.
*/
type RosenbrockServer struct {
	*runtime.System
	x []float64
	fVal float64
	iterations int32
}

func NewRosenbrock(ctx context.Context) *RosenbrockServer {
	server := &RosenbrockServer{
		System: runtime.NewSystem(ctx, "optimization.rosenbrock"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write processes incoming observation and optimization problem definition.
*/
func (server *RosenbrockServer) Write(ctx context.Context, call Rosenbrock_write) error {
	args := call.Args()
	paramA := args.A()
	paramB := args.B()
	initList, err := args.InitX()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to read initX", err))
	}

	initX := make([]float64, 2)
	if initList.Len() >= 2 {
		initX[0] = initList.At(0)
		initX[1] = initList.At(1)
	}

	problem := optimize.Problem{
		Func: func(point []float64) float64 {
			diffA := paramA - point[0]
			diffB := point[1] - point[0]*point[0]
			return diffA*diffA + paramB*diffB*diffB
		},
		Grad: func(grad, point []float64) {
			diffA := paramA - point[0]
			diffB := point[1] - point[0]*point[0]
			grad[0] = -2.0*diffA - 4.0*paramB*point[0]*diffB
			grad[1] = 2.0*paramB*diffB
		},
	}

	res, err := optimize.Minimize(problem, initX, nil, &optimize.BFGS{})

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
func (server *RosenbrockServer) Done(ctx context.Context, call Rosenbrock_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "[optimization.rosenbrock.Done] failed to allocate results", err))
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
