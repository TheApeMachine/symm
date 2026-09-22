package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestLatinHypercube(t *testing.T) {
	Convey("Given a LatinHypercube server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewLatinHypercube(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.LatinHypercube_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.LatinHypercube_write_Params) error {
				params.SetCount(5)
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
