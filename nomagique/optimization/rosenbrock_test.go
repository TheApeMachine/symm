package optimization_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/optimization"
)

func TestRosenbrock(t *testing.T) {
	Convey("Given a Rosenbrock server and client", t, func() {
		ctx := context.Background()
		server := optimization.NewRosenbrock(ctx)
		So(server, ShouldNotBeNil)

		client := optimization.Rosenbrock_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params optimization.Rosenbrock_write_Params) error {
				listInitX, err := params.NewInitX(2)

				if err != nil {
					return err
				}
				listInitX.Set(0, 0.0)
				listInitX.Set(1, 0.0)
				params.SetA(1.0)
				params.SetB(100.0)
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
