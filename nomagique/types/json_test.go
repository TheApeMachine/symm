package types

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestJSONServer(t *testing.T) {
	Convey("Given a JSONServer node", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewJSON(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capJSON := JSON_ServerToClient(server)
		So(capJSON.IsValid(), ShouldBeTrue)

		Convey("When writing JSON text via Write", func() {
			subscriptionText := `{"event":"subscribe","pair":["BTC/USD"]}`
			err := capJSON.Write(ctx, func(params JSON_write_Params) error {
				return params.SetText(subscriptionText)
			})
			So(err, ShouldBeNil)

			Convey("Then Done emits the marshaled JSON bytes", func() {
				future, release := capJSON.Done(ctx, nil)
				defer release()

				results, err := future.Struct()
				So(err, ShouldBeNil)

				out, err := results.Out()
				So(err, ShouldBeNil)
				So(string(out), ShouldEqual, `{"event":"subscribe","pair":["BTC/USD"]}`)
			})
		})

		Convey("When initialized with pre-marshaled bytes", func() {
			initial := []byte(`{"event":"ping"}`)
			serverWithBytes := NewJSONWithBytes(ctx, initial)
			capWithBytes := JSON_ServerToClient(serverWithBytes)

			future, release := capWithBytes.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, `{"event":"ping"}`)
		})
	})
}
