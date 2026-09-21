package transport_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTransportPrimitives(t *testing.T) {
	ctx := context.Background()

	Convey("Given transport primitives", t, func() {
		Convey("Process streams data through Done", func() {
			server := transport.NewProcess(t.Context())
			client := transport.Process_ServerToClient(server)
			So(client.IsValid(), ShouldBeTrue)

			err := client.Write(ctx, func(params transport.Process_write_Params) error {
				params.SetData([]byte("kraken_payload"))
				params.SetBinary("kraken")
				params.SetArgs("paper")
				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "kraken_payload")
		})
	})
}
