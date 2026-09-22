package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestWithoutReplacement(t *testing.T) {
	Convey("Given a WithoutReplacement server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewWithoutReplacement(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.WithoutReplacement_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.WithoutReplacement_write_Params) error {
				params.SetCount(3)
				params.SetN(10)
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
