package optimization

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
	"gonum.org/v1/gonum/floats"
)

/*
Constants of the named algorithm this node implements: the Armijo
sufficient-decrease coefficient and the backtracking contraction of the
L-BFGS line search (Nocedal & Wright, Numerical Optimization, Algorithm 3.1
and Algorithm 7.5). They belong to the algorithm's definition, not to any
problem it is pointed at.
*/
const (
	armijoDecrease  = 1e-4
	backtrackFactor = 0.5
)

/*
LBFGSServer steps the limited-memory BFGS iteration. It never evaluates an
objective: each Write carries the value and gradient measured at the point
the previous Done proposed, and each Done answers with the next point to
measure. The objective is a separate node and the graph closes the loop, so
this node stays free of any knowledge of what is being minimized.
*/
type LBFGSServer struct {
	*runtime.System

	memory    int
	tolerance float64

	point    []float64
	value    float64
	gradient []float64
	started  bool

	direction []float64
	step      float64
	slope     float64

	deltaPoint    [][]float64
	deltaGradient [][]float64

	proposal   []float64
	iterations int32
	converged  bool
}

func NewLBFGS(ctx context.Context) *LBFGSServer {
	server := &LBFGSServer{
		System: runtime.NewSystem(ctx, "optimization.lbfgs"),
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write accepts the objective value and gradient at the current point and
advances the iteration by one step, leaving the next point to evaluate in
the proposal.
*/
func (server *LBFGSServer) Write(ctx context.Context, call LBFGS_write) error {
	args := call.Args()
	gradient, err := server.readList(args.Gradient())

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "lbfgs: failed to read gradient", err))
	}

	seed, err := server.readList(args.Seed())

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "lbfgs: failed to read seed", err))
	}

	server.absorb(args)

	// The value answers the point last proposed, so until one has been
	// proposed there is nothing for it to answer and the seed is where the
	// search has to start.
	if len(server.proposal) == 0 {
		server.proposal = seed
		return nil
	}

	if server.converged {
		return nil
	}

	point := server.proposal

	if len(gradient) != len(point) {
		return nil
	}

	if !server.started {
		server.begin(point, args.FVal(), gradient)
		return nil
	}

	if args.FVal() <= server.value+armijoDecrease*server.step*server.slope {
		server.accept(point, args.FVal(), gradient)
		return nil
	}

	server.backtrack()
	return nil
}

/*
absorb records the memory depth and stopping tolerance the caller derived
for this problem.
*/
func (server *LBFGSServer) absorb(args LBFGS_write_Params) {
	server.memory = int(args.Memory())
	server.tolerance = args.Tolerance()
}

/*
begin seeds the iteration from the first measured point, taking the steepest
descent direction with the scale-free initial step 1/||g||.
*/
func (server *LBFGSServer) begin(point []float64, value float64, gradient []float64) {
	server.point = point
	server.value = value
	server.gradient = gradient
	server.started = true

	if server.settled(gradient) {
		return
	}

	server.direction = server.steepest(gradient)
	server.slope = floats.Dot(gradient, server.direction)
	server.step = 1 / floats.Norm(gradient, math.Inf(1))
	server.advance()
}

/*
accept takes a point that met the sufficient-decrease condition, folds the
curvature pair it produced into the limited-memory history, and proposes the
next point along the refreshed direction.
*/
func (server *LBFGSServer) accept(point []float64, value float64, gradient []float64) {
	deltaPoint := make([]float64, len(point))
	floats.SubTo(deltaPoint, point, server.point)
	deltaGradient := make([]float64, len(gradient))
	floats.SubTo(deltaGradient, gradient, server.gradient)

	server.point = point
	server.value = value
	server.gradient = gradient
	server.iterations++

	if server.settled(gradient) {
		return
	}

	server.remember(deltaPoint, deltaGradient)
	server.direction = server.twoLoop(gradient)
	server.slope = floats.Dot(gradient, server.direction)

	if server.slope >= 0 {
		server.direction = server.steepest(gradient)
		server.slope = floats.Dot(gradient, server.direction)
	}

	server.step = 1
	server.advance()
}

/*
backtrack contracts the step along the current direction after a trial point
failed the sufficient-decrease condition, and stops the iteration once the
step has shrunk past the caller's tolerance.
*/
func (server *LBFGSServer) backtrack() {
	server.step *= backtrackFactor

	if server.step <= server.tolerance {
		server.converged = true
		server.proposal = server.point
		return
	}

	server.advance()
}

/*
settled reports whether the gradient has fallen inside the caller's
tolerance, and parks the iteration on the current point when it has.
*/
func (server *LBFGSServer) settled(gradient []float64) bool {
	if floats.Norm(gradient, math.Inf(1)) > server.tolerance {
		return false
	}

	server.converged = true
	server.proposal = server.point
	return true
}

/*
steepest returns the negated gradient, the fallback direction whenever the
limited-memory model does not offer a descent direction.
*/
func (server *LBFGSServer) steepest(gradient []float64) []float64 {
	direction := make([]float64, len(gradient))

	for index, value := range gradient {
		direction[index] = -value
	}

	return direction
}

/*
advance writes the next point to evaluate: the accepted point displaced along
the current direction by the current step.
*/
func (server *LBFGSServer) advance() {
	proposal := make([]float64, len(server.point))

	for index, value := range server.point {
		proposal[index] = value + server.step*server.direction[index]
	}

	server.proposal = proposal
}

/*
remember folds one curvature pair into the limited-memory history, keeping
only pairs that preserve positive definiteness and only as many as the
caller's memory depth allows.
*/
func (server *LBFGSServer) remember(deltaPoint, deltaGradient []float64) {
	if floats.Dot(deltaGradient, deltaPoint) <= 0 {
		return
	}

	server.deltaPoint = append(server.deltaPoint, deltaPoint)
	server.deltaGradient = append(server.deltaGradient, deltaGradient)

	if server.memory <= 0 || len(server.deltaPoint) <= server.memory {
		return
	}

	server.deltaPoint = server.deltaPoint[len(server.deltaPoint)-server.memory:]
	server.deltaGradient = server.deltaGradient[len(server.deltaGradient)-server.memory:]
}

/*
twoLoop applies the L-BFGS two-loop recursion, returning the search direction
-H*g implied by the retained curvature pairs.
*/
func (server *LBFGSServer) twoLoop(gradient []float64) []float64 {
	depth := len(server.deltaPoint)

	if depth == 0 {
		return server.steepest(gradient)
	}

	direction := append([]float64(nil), gradient...)
	alpha := make([]float64, depth)
	rho := make([]float64, depth)

	for index := depth - 1; index >= 0; index-- {
		rho[index] = 1 / floats.Dot(server.deltaGradient[index], server.deltaPoint[index])
		alpha[index] = rho[index] * floats.Dot(server.deltaPoint[index], direction)
		floats.AddScaled(direction, -alpha[index], server.deltaGradient[index])
	}

	last := depth - 1
	scale := floats.Dot(server.deltaPoint[last], server.deltaGradient[last]) /
		floats.Dot(server.deltaGradient[last], server.deltaGradient[last])
	floats.Scale(scale, direction)

	for index := 0; index < depth; index++ {
		beta := rho[index] * floats.Dot(server.deltaGradient[index], direction)
		floats.AddScaled(direction, alpha[index]-beta, server.deltaPoint[index])
	}

	floats.Scale(-1, direction)
	return direction
}

/*
readList copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *LBFGSServer) readList(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

/*
Done answers with the next point the objective should be evaluated at, the
best value reached so far, and whether the iteration has settled.
*/
func (server *LBFGSServer) Done(ctx context.Context, call LBFGS_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "lbfgs: failed to allocate results", err))
	}

	listX, err := results.NewX(int32(len(server.proposal)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "lbfgs: failed to allocate x list", err))
	}

	for index, value := range server.proposal {
		listX.Set(index, value)
	}

	results.SetFVal(server.value)
	results.SetIterations(server.iterations)
	results.SetConverged(server.converged)
	return nil
}
