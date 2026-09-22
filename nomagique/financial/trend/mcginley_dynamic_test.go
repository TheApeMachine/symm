package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestMcGinleyDynamic(t *testing.T) {
	Convey("Given a McGinleyDynamic server and client", t, func() {
		ctx := context.Background()
		server := trend.NewMcGinleyDynamic(ctx)
		So(server, ShouldNotBeNil)

		client := trend.McGinleyDynamic_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params trend.McGinleyDynamic_write_Params) error {
				params.SetValue(100.0)
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
