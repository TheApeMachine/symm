package data_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestProjectionNext(t *testing.T) {
	Convey("Declared paths become measurements without inventing gated metrics", t, func() {
		at := time.Unix(1700000000, 0).UTC()
		projection := &data.Projection{
			Source:   "test",
			Identity: func() (string, string, time.Time, time.Time) { return "id", "BTC/USD", at, at },
			Metrics: []data.MetricProjection{
				{Path: []string{"alpha"}, Label: "alpha", Unit: data.UnitRate},
				{Path: []string{"beta"}, Label: "beta", Defined: []string{"beta_defined"}},
			},
			Facts: []data.FactProjection{
				{Name: data.MetadataSupport, Path: []string{"support"}},
				{Name: data.MetadataDivergence, Path: []string{"residual"}},
				{Name: data.MetadataNoiseVariance, Path: []string{"variance"}},
			},
		}

		for _, value := range []float64{2.5, 0, -1} {
			measurement, err := transport.Evaluate(projection, transport.Values(data.ProjectionInput{
				Values: map[string]float64{"alpha": value, "support": 4, "residual": 2, "variance": 1},
				Flags:  map[string]bool{"beta_defined": false},
			}))
			So(err, ShouldBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.ID, ShouldEqual, "id")
			So(measurement.Metrics["alpha"].Raw, ShouldEqual, value)
			So(measurement.Maturity, ShouldEqual, 0.75)
			So(measurement.SNR, ShouldEqual, 4)
			_, exists := measurement.Metrics["beta"]
			So(exists, ShouldBeFalse)
		}
	})
}
