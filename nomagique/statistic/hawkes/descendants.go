package hawkes

import (
	"context"
	"math"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/mat"
)

/*
DescendantsServer returns the expected total progeny of one arrival on each
component, counted over every generation rather than only the first. Summing
the geometric series of the branching matrix gives the inverse of (identity -
branching); its column sums count the parent's whole family tree, and
dropping the parent itself leaves the descendants.
*/
type DescendantsServer struct {
	descendants []float64
	defined     bool
}

func NewDescendants() *DescendantsServer {
	return &DescendantsServer{}
}

func (server *DescendantsServer) Write(ctx context.Context, call Descendants_write) error {
	args := call.Args()
	branching, err := server.read(args.Branching())

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes descendants: failed to read branching",
			err,
		))
	}

	dimension := int(args.Dimension())
	server.descendants = nil
	server.defined = false

	if dimension <= 0 || len(branching) < dimension*dimension {
		return nil
	}

	residual := mat.NewDense(dimension, dimension, nil)

	for row := 0; row < dimension; row++ {
		for column := 0; column < dimension; column++ {
			identity := 0.0

			if row == column {
				identity = 1
			}

			residual.Set(row, column, identity-branching[row*dimension+column])
		}
	}

	var inverse mat.Dense

	if err := inverse.Inverse(residual); err != nil {
		return nil
	}

	descendants := make([]float64, dimension)

	for column := 0; column < dimension; column++ {
		total := 0.0

		for row := 0; row < dimension; row++ {
			total += inverse.At(row, column)
		}

		descendants[column] = total - 1

		if descendants[column] < 0 || math.IsNaN(descendants[column]) || math.IsInf(descendants[column], 0) {
			return nil
		}
	}

	server.descendants = descendants
	server.defined = true
	return nil
}

/*
read copies a Cap'n Proto float list into caller-owned storage.
*/
func (server *DescendantsServer) read(list capnp.Float64List, err error) ([]float64, error) {
	if err != nil {
		return nil, err
	}

	values := make([]float64, list.Len())

	for index := range values {
		values[index] = list.At(index)
	}

	return values, nil
}

func (server *DescendantsServer) Done(ctx context.Context, call Descendants_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes descendants: failed to allocate results",
			err,
		))
	}

	list, err := results.NewDescendants(int32(len(server.descendants)))

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"hawkes descendants: failed to allocate descendants list",
			err,
		))
	}

	for index, value := range server.descendants {
		list.Set(index, value)
	}

	results.SetDefined(server.defined)
	return nil
}
