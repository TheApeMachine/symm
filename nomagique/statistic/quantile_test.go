package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestQuantile(t *testing.T) {
	Convey("Given a Quantile server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewQuantile(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.Quantile_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.Quantile_write_Params) error {
				listValues, err := params.NewValues(5)

				if err != nil {
					return err
				}
				listValues.Set(0, 1.0)
				listValues.Set(1, 2.0)
				listValues.Set(2, 3.0)
				listValues.Set(3, 4.0)
				listValues.Set(4, 5.0)
				params.SetP(0.5)
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
