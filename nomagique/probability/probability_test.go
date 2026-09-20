package probability_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestProbabilityPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given probability primitives", t, func() {
		Convey("Entropy accumulates negative p log p and resets on Done", func() {
			server := probability.NewEntropy()
			client := probability.Entropy_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			p := 0.5
			err := client.Write(ctx, func(params probability.Entropy_write_Params) error {
				params.SetIn(p)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			expected := -0.5 * math.Log(0.5)
			So(results.Out(), ShouldAlmostEqual, expected, 1e-9)

			Convey("When evaluating again, state was reset", func() {
				err = client.Write(ctx, func(params probability.Entropy_write_Params) error {
					params.SetIn(1.0)
					return nil
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)

				secondFuture, secondRelease := client.Done(ctx, nil)
				defer secondRelease()

				secondResults, err := secondFuture.Struct()
				So(err, ShouldBeNil)
				So(secondResults.Out(), ShouldEqual, 0.0)
			})
		})

		Convey("Concentration computes square from Float64", func() {
			server := probability.NewConcentration()
			client := probability.Concentration_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params probability.Concentration_write_Params) error {
				params.SetIn(0.5)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Out(), ShouldAlmostEqual, 0.25, 1e-9)
		})

		Convey("Distribution computes ambiguity, confidence, and returns scalar fields", func() {
			server := probability.NewDistribution()
			client := probability.Distribution_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params probability.Distribution_write_Params) error {
				params.SetIn(0.7)
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Winner(), ShouldEqual, 0)
			So(results.Confidence(), ShouldAlmostEqual, 0.7, 1e-9)
			So(results.Out(), ShouldAlmostEqual, 0.7, 1e-9)
			So(results.Ambiguity(), ShouldAlmostEqual, 0.3, 1e-9)
		})
	})
}
