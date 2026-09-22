package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestGeometricMean(t *testing.T) {
	Convey("Given a GeometricMean server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewGeometricMean(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.GeometricMean_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.GeometricMean_write_Params) error {
				params.SetValue(10.0)
				return nil
			})
			So(err, ShouldBeNil)

			err = client.WaitStreaming()
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.IsValid(), ShouldBeTrue)
		})
	})
}
