package geometry

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
)

/*
InversionServer turns signed relationship strengths into target distances.
*/
type InversionServer struct {
	distance []float64
}

func NewInversion() *InversionServer {
	return &InversionServer{}
}

func (server *InversionServer) Write(ctx context.Context, call Inversion_write) error {
	strength, err := call.Args().Strength()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "geometry.inversion: failed to read strength", err))
	}

	server.distance = make([]float64, strength.Len())

	// Strength has no unit of its own: it is read against the root mean
	// square of the strengths read now, so a typical relationship asks for a
	// typical distance whatever the scale the evidence happens to sit at.
	squares := 0.0

	for edge := range strength.Len() {
		squares += strength.At(edge) * strength.At(edge)
	}

	if squares == 0 {
		for edge := range server.distance {
			server.distance[edge] = 1
		}

		return nil
	}

	scale := math.Sqrt(squares / float64(strength.Len()))

	for edge := range strength.Len() {
		server.distance[edge] = invert(strength.At(edge) / scale)
	}

	return nil
}

/*
invert is the target distance one strength asks for, in lattice cells.
*/
func invert(strength float64) float64 {
	if strength > 0 {
		return 1 / (1 + strength)
	}

	return 1 - strength
}

func (server *InversionServer) Done(ctx context.Context, call Inversion_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.inversion: failed to allocate results", err))
	}

	distance, err := results.NewDistance(int32(len(server.distance)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "geometry.inversion: failed to allocate distance", err))
	}

	for edge, value := range server.distance {
		distance.Set(edge, value)
	}

	server.distance = nil
	return nil
}
