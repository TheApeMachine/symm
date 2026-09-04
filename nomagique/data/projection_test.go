package data

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

/* reporting publishes one declared reading about itself. */
type reporting struct {
	label   string
	value   types.Scalar
	defined bool
}

func (node *reporting) Step(x types.Scalar) types.Scalar { return x }

func (node *reporting) Readings() []types.Reading {
	return []types.Reading{{
		Label:     node.label,
		Unit:      "rate",
		Timescale: "instantaneous",
		Value:     node.value,
		Defined:   node.defined,
	}}
}

/* evidencing declares the confidence its estimate carries. */
type evidencing struct{ reporting }

func (node *evidencing) Support() float64            { return 4 }
func (node *evidencing) Divergence() types.Scalar    { return 2 }
func (node *evidencing) NoiseVariance() types.Scalar { return 1 }

/* rejecting declares an observation the composition could not measure. */
type rejecting struct{ reject bool }

func (node *rejecting) Step(x types.Scalar) types.Scalar { return x }
func (node *rejecting) Rejected() bool                   { return node.reject }

var errUnmeasurable = errors.New("unmeasurable")

func TestProjection(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()

	Convey("Given a projection terminating a composition", t, func() {
		projection := &Projection{
			Source: "test",
			Identity: func() (string, string, time.Time, time.Time) {
				return "BTC/USD:test:1", "BTC/USD", at, at
			},
		}

		root := &types.Chain{
			A: &reporting{label: "alpha", value: 2.5, defined: true},
			B: &reporting{label: "beta", value: -1, defined: true},
			C: projection,
		}

		types.Bind(root, &types.Tick{})

		Convey("it harvests every upstream reading without being handed one", func() {
			root.Step(0)

			measurement := projection.Measurement()
			So(measurement, ShouldNotBeNil)
			So(measurement.ID, ShouldEqual, "BTC/USD:test:1")
			So(measurement.Metrics["alpha"].Raw, ShouldEqual, 2.5)
			So(measurement.Metrics["beta"].Raw, ShouldEqual, -1.0)
			So(measurement.Metrics["alpha"].Unit, ShouldEqual, UnitRate)
		})
	})

	Convey("Given an undefined upstream reading", t, func() {
		projection := &Projection{Source: "test"}
		root := &types.Chain{
			A: &reporting{label: "undefined", value: 5, defined: false},
			B: &reporting{label: "defined", value: 5, defined: true},
			C: projection,
		}

		types.Bind(root, &types.Tick{})

		Convey("it is absent rather than published as a zero", func() {
			root.Step(0)

			measurement := projection.Measurement()
			_, present := measurement.Metrics["undefined"]
			So(present, ShouldBeFalse)
			So(measurement.Metrics["defined"].Raw, ShouldEqual, 5.0)
		})
	})

	Convey("Given an upstream stage declaring its evidence", t, func() {
		projection := &Projection{Source: "test"}
		root := &types.Chain{
			A: &evidencing{reporting{label: "alpha", value: 1, defined: true}},
			B: projection,
		}

		types.Bind(root, &types.Tick{})

		Convey("Finalize derives maturity and SNR from the declared facts", func() {
			root.Step(0)

			measurement := projection.Measurement()
			So(measurement.Metadata[MetadataSupport], ShouldEqual, 4.0)
			So(measurement.Maturity, ShouldEqual, 0.75)
			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldEqual, 4.0)
		})
	})

	Convey("Given an upstream stage rejecting the observation", t, func() {
		projection := &Projection{Source: "test", Rejection: errUnmeasurable}
		root := &types.Chain{
			A: &rejecting{reject: true},
			B: &reporting{label: "alpha", value: 1, defined: true},
			C: projection,
		}

		types.Bind(root, &types.Tick{})

		Convey("the measurement carries the failure, not readings", func() {
			root.Step(0)

			measurement := projection.Measurement()
			So(measurement.Err, ShouldEqual, errUnmeasurable)
			So(measurement.Metrics, ShouldBeEmpty)
		})
	})

	Convey("Given a projection bound to nothing", t, func() {
		projection := &Projection{Source: "test"}

		Convey("it publishes an empty measurement rather than failing", func() {
			projection.Step(0)
			So(projection.Measurement(), ShouldNotBeNil)
			So(projection.Measurement().Metrics, ShouldBeEmpty)
		})
	})
}
