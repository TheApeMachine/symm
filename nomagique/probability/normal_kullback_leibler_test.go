package probability_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestNormalKullbackLeibler(t *testing.T) {
	Convey("Given a NormalKullbackLeibler server and client", t, func() {
		ctx := context.Background()
		server := probability.NewNormalKullbackLeibler(ctx)
		So(server, ShouldNotBeNil)

		client := probability.NormalKullbackLeibler_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params probability.NormalKullbackLeibler_write_Params) error {
				params.SetMuL(0.0)
				params.SetSigmaL(1.0)
				params.SetMuR(1.0)
				params.SetSigmaR(1.0)
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
