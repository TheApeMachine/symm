package differentiation_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/differentiation"
)

func TestFiniteDifference(t *testing.T) {
	Convey("Given a FiniteDifference server and client", t, func() {
		ctx := context.Background()
		server := differentiation.NewFiniteDifference(ctx)
		So(server, ShouldNotBeNil)

		client := differentiation.FiniteDifference_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params differentiation.FiniteDifference_write_Params) error {
				listValues, err := params.NewValues(5)

				if err != nil {
					return err
				}
				listValues.Set(0, 1.0)
				listValues.Set(1, 4.0)
				listValues.Set(2, 9.0)
				listValues.Set(3, 16.0)
				listValues.Set(4, 25.0)
				params.SetStep(1.0)
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
