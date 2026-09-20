package data_test

import (
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDataPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given data primitives", t, func() {
		Convey("Extract extracts raw metric value from Data", func() {
			server := data.NewExtract()
			client := data.Extract_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			So(err, ShouldBeNil)

			measurement, err := data.NewRootWireMeasurement(seg)
			So(err, ShouldBeNil)

			metrics, err := measurement.NewMetrics(1)
			So(err, ShouldBeNil)
			metrics.At(0).SetRaw(42.5)

			dataBytes, err := msg.Marshal()
			So(err, ShouldBeNil)

			err = client.Write(ctx, func(params data.Extract_write_Params) error {
				params.SetPath("price")
				return params.SetIn(dataBytes)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldEqual, 42.5)

			Convey("When evaluating second observation, state is reset", func() {
				secondMsg, secondSeg, err := capnp.NewMessage(capnp.SingleSegment(nil))
				So(err, ShouldBeNil)

				secondM, err := data.NewRootWireMeasurement(secondSeg)
				So(err, ShouldBeNil)

				secondMetrics, err := secondM.NewMetrics(1)
				So(err, ShouldBeNil)
				secondMetrics.At(0).SetRaw(100.0)

				secondBytes, err := secondMsg.Marshal()
				So(err, ShouldBeNil)

				err = client.Write(ctx, func(params data.Extract_write_Params) error {
					return params.SetIn(secondBytes)
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, 100.0)
			})
		})

		Convey("Quality computes SNR and maturity from scalar fields", func() {
			server := data.NewQuality()
			client := data.Quality_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params data.Quality_write_Params) error {
				params.SetSupport(10.0)
				params.SetDivergence(2.0)
				params.SetNoiseVariance(1.0)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			So(results.Estimated(), ShouldBeTrue)
			So(results.SnrDefined(), ShouldBeTrue)
			So(results.Snr(), ShouldEqual, 4.0)
			So(results.Maturity(), ShouldAlmostEqual, 0.9, 1e-6)
		})
	})
}
