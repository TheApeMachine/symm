package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestTriangle(t *testing.T) {
	Convey("Given a Triangle server and client", t, func() {
		ctx := context.Background()
		server := probability.NewTriangle(ctx)
		So(server, ShouldNotBeNil)

		client := probability.Triangle_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.Triangle_write_Params) error {
				params.SetA(0.0)
				params.SetB(2.0)
				params.SetC(1.0)
				params.SetX(1.0)
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
