package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestWeighted(t *testing.T) {
	Convey("Given a Weighted server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewWeighted(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.Weighted_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.Weighted_write_Params) error {
				listWeights, err := params.NewWeights(4)

				if err != nil {
					return err
				}
				listWeights.Set(0, 1.0)
				listWeights.Set(1, 2.0)
				listWeights.Set(2, 3.0)
				listWeights.Set(3, 4.0)
				params.SetCount(2)
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
