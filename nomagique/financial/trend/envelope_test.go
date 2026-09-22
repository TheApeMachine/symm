package trend_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/financial/trend"
)

func TestEnvelope(t *testing.T) {
	Convey("Given a Envelope server and client", t, func() {
		ctx := context.Background()
		server := trend.NewEnvelope(ctx)
		So(server, ShouldNotBeNil)

		client := trend.Envelope_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params trend.Envelope_write_Params) error {
				params.SetClose(100.0)
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
