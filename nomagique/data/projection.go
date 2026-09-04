package data

import (
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Projection is the terminal stage of a composition: it names what the pipeline
measured as a Measurement.

It is passive. It holds no readings of its own and is handed no pointers to
the stages above it. At construction the builder binds it to the composition
it terminates, and on each Step it harvests the readings every upstream node
publishes about itself.

That is what makes the composition the definition: a stage states what it
measured, and nothing outside the graph has to know how to ask.

Identity names the observation being published. It is a function because
identity is a name, not a quantity, and the carrier only carries quantities.
*/
type Projection struct {
	Source   string
	Identity func() (id string, label string, at time.Time, from time.Time)

	// Rejection is the failure this projection reports when an upstream stage
	// declares the observation unmeasurable.
	Rejection error

	upstream    []types.Node
	measurement *Measurement[float64]
}

/*
Bind attaches the composition this projection terminates. The builder calls it;
a consumer never does.
*/
func (projection *Projection) Bind(root types.Node) {
	projection.upstream = nil

	types.Walk(root, func(node types.Node) {
		if node == projection {
			return
		}

		projection.upstream = append(projection.upstream, node)
	})
}

/*
Step harvests every upstream reading and publishes the Measurement. It passes
the carrier through unchanged, so terminating a Chain with a Projection does
not alter what the composition computes.
*/
func (projection *Projection) Step(x types.Scalar) types.Scalar {
	var (
		id    string
		label string
		at    time.Time
		from  time.Time
	)

	if projection.Identity != nil {
		id, label, at, from = projection.Identity()
	}

	measurement := NewMeasurement[float64](id, label, projection.Source, at, from)

	for _, node := range projection.upstream {
		if rejector, ok := node.(types.Rejector); ok && rejector.Rejected() {
			measurement.Err = projection.Rejection
			projection.measurement = measurement

			return x
		}
	}

	metadata := map[string]float64{}

	for _, node := range projection.upstream {
		reporter, ok := node.(types.Reporter)

		if !ok {
			continue
		}

		for _, reading := range reporter.Readings() {
			if !reading.Defined || reading.Label == "" {
				continue
			}

			measurement.PutMetric(Metric[float64]{
				Label:     reading.Label,
				Raw:       float64(reading.Value),
				Unit:      Unit(reading.Unit),
				Timescale: Timescale(reading.Timescale),
			})
		}

		if evidence, ok := node.(types.Evidence); ok {
			metadata[MetadataSupport] = evidence.Support()
			metadata[MetadataDivergence] = float64(evidence.Divergence())
			metadata[MetadataNoiseVariance] = float64(evidence.NoiseVariance())
		}
	}

	if len(metadata) > 0 {
		measurement.Metadata = metadata
	}

	measurement.Finalize()
	projection.measurement = measurement

	return x
}

// Measurement returns the Measurement the most recent Step published.
func (projection *Projection) Measurement() *Measurement[float64] {
	return projection.measurement
}

var _ types.Node = (*Projection)(nil)
