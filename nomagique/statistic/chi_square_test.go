package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestChiSquare(t *testing.T) {
	Convey("Given a ChiSquare server and client", t, func() {
		ctx := context.Background()
		server := statistic.NewChiSquare(ctx)
		So(server, ShouldNotBeNil)

		client := statistic.ChiSquare_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params statistic.ChiSquare_write_Params) error {
				listObserved, err := params.NewObserved(3)

				if err != nil {
					return err
				}
				listObserved.Set(0, 10.0)
				listObserved.Set(1, 20.0)
				listObserved.Set(2, 30.0)
				listExpected, err := params.NewExpected(3)

				if err != nil {
					return err
				}
				listExpected.Set(0, 12.0)
				listExpected.Set(1, 18.0)
				listExpected.Set(2, 30.0)
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
