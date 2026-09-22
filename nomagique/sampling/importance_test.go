package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestImportance(t *testing.T) {
	Convey("Given a Importance server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewImportance(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.Importance_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.Importance_write_Params) error {
				params.SetCount(5)
				params.SetTargetMu(0.0)
				params.SetTargetSigma(1.0)
				params.SetPropMu(0.0)
				params.SetPropSigma(2.0)
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
