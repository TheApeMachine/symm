package hawkes

import (
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
IntensityServer evaluates the conditional intensity of each component: the
rate the process is running at right now, given everything that has already
happened. It is the component's own baseline plus whatever excitation the
past has left standing, weighted by how strongly each component drives it.
*/
type IntensityServer struct {
	intensity []float64
}

func NewIntensity() *IntensityServer {
	return &IntensityServer{}
}

func (server *IntensityServer) Write(ctx context.Context, call Intensity_write) error {
	args := call.Args()
	baseline, err := server.read(args.Baseline())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes intensity: failed to read baseline",
			err,
		))
	}

	excitation, err := server.read(args.Excitation())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes intensity: failed to read excitation",
			err,
		))
	}

	support, err := server.read(args.Support())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes intensity: failed to read support",
			err,
		))
	}

	dimension := int(args.Dimension())
	server.intensity = nil

	if dimension <= 0 || len(baseline) < dimension || len(support) < dimension {
		return nil
	}

	if len(excitation) < dimension*dimension {
		return nil
	}

	intensity := make([]float64, dimension)

	for row := 0; row < dimension; row++ {
		total := baseline[row]

		for column := 0; column < dimension; column++ {
			total += excitation[row*dimension+column] * support[column]
		}

		intensity[row] = total
	}

	server.intensity = intensity
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *IntensityServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *IntensityServer) Done(ctx context.Context, call Intensity_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes intensity: failed to allocate results",
			err,
		))
	}

	list, err := results.NewIntensity(int32(len(server.intensity)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes intensity: failed to allocate intensity list",
			err,
		))
	}

	for index, value := range server.intensity {
		list.Set(index, value)
	}

	return nil
}
