package sampling_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/sampling"
)

func TestMetropolisHastings(t *testing.T) {
	Convey("Given a MetropolisHastings server and client", t, func() {
		ctx := context.Background()
		server := sampling.NewMetropolisHastings(ctx)
		So(server, ShouldNotBeNil)

		client := sampling.MetropolisHastings_ServerToClient(server)
		So(client.IsValid(), ShouldBeTrue)

		Convey("When writing input values", func() {
			err := client.Write(ctx, func(params sampling.MetropolisHastings_write_Params) error {
				params.SetCount(5)
				params.SetInitial(0.0)
				params.SetBurnIn(10)
				params.SetRate(2)
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
