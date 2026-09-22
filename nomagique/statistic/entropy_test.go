package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestEntropy(t *testing.T) {
	Convey("Given a Entropy server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewEntropy(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.Entropy_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.Entropy_write_Params) error {
				listP, err := params.NewP(2)

				if err != nil {
					return err
				}
				listP.Set(0, 0.5)
				listP.Set(1, 0.5)
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
