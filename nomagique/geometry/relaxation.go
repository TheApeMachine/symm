package geometry

import (
	"context"
	"fmt"
	"math"

	"github.com/theapemachine/errnie"
)

/*
RelaxationServer moves points on a plane toward the distances their
relationships ask for.
*/
type RelaxationServer struct {
	positions []float64
}

func NewRelaxation() *RelaxationServer {
	return &RelaxationServer{}
}

func (server *RelaxationServer) Write(ctx context.Context, call Relaxation_write) error {
	args := call.Args()
	positions, err := args.Positions()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read positions", err))
	}

	known, err := args.Known()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read known", err))
	}

	authority, err := args.Authority()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read authority", err))
	}

	from, err := args.FromNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read fromNodes", err))
	}

	to, err := args.ToNodes()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read toNodes", err))
	}

	strength, err := args.Strength()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read strength", err))
	}

	distance, err := args.Distance()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.relaxation: failed to read distance", err))
	}

	edges := from.Len()

	if positions.Len() != known.Len()*2 || to.Len() != edges || strength.Len() != edges || distance.Len() != edges {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"geometry.relaxation: %d positions for %d points; %d edges with %d ends, %d strengths, %d distances",
				positions.Len(), known.Len(), edges, to.Len(), strength.Len(), distance.Len(),
			),
			nil,
		))
	}

	count := authority.Len()
	width := int(math.Ceil(math.Sqrt(float64(count))))
	server.positions = make([]float64, count*2)

	for point := range count {
		server.positions[point*2] = float64(point % width)
		server.positions[point*2+1] = float64(point / width)

		if point >= known.Len() || !known.At(point) {
			continue
		}

		server.positions[point*2] = positions.At(point * 2)
		server.positions[point*2+1] = positions.At(point*2 + 1)
	}

	for edge := range edges {
		left, right := int(from.At(edge)), int(to.At(edge))

		if left < 0 || right < 0 || left >= count || right >= count {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("geometry.relaxation: edge %d-%d is outside %d points", left, right, count),
				nil,
			))
		}

		server.relax(left, right, authority.At(left), authority.At(right), strength.At(edge), distance.At(edge), count)
	}

	return nil
}

/*
relax takes one stress step on one relationship.
*/
func (server *RelaxationServer) relax(left, right int, leftAuthority, rightAuthority, strength, target float64, count int) {
	combined := leftAuthority + rightAuthority

	if combined == 0 || strength == 0 {
		return
	}

	horizontal := server.positions[right*2] - server.positions[left*2]
	vertical := server.positions[right*2+1] - server.positions[left*2+1]
	current := math.Hypot(horizontal, vertical)
	residual := current - target

	// Repulsion is a least separation, not a spring: inconsistency pushes a
	// pair apart while it is closer than its target and never draws together
	// a pair that is already farther apart.
	if strength < 0 && residual >= 0 {
		return
	}

	if current == 0 {
		// Coincident points have no direction between them. The angle names
		// the right-hand point on the unit circle, so repulsion separates
		// them deterministically rather than along one shared axis.
		angle := 2 * math.Pi * float64(right) / float64(count)
		horizontal, vertical, current = math.Cos(angle), math.Sin(angle), 1
	}

	pull := math.Abs(strength) * combined
	correction := pull / (1 + pull) * residual / current
	leftShare := rightAuthority / combined
	rightShare := leftAuthority / combined

	server.positions[left*2] += correction * horizontal * leftShare
	server.positions[left*2+1] += correction * vertical * leftShare
	server.positions[right*2] -= correction * horizontal * rightShare
	server.positions[right*2+1] -= correction * vertical * rightShare
}

func (server *RelaxationServer) Done(ctx context.Context, call Relaxation_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.relaxation: failed to allocate results", err))
	}

	positions, err := results.NewPositions(int32(len(server.positions)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.relaxation: failed to allocate positions", err))
	}

	for offset, value := range server.positions {
		positions.Set(offset, value)
	}

	index, err := results.NewIndex(int32(len(server.positions) / 2))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.relaxation: failed to allocate index", err))
	}

	for point := range len(server.positions) / 2 {
		index.Set(point, int64(point))
	}

	server.positions = nil
	return nil
}
