package webrtc

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestWebRTCServer(t *testing.T) {
	Convey("Given a WebRTCServerServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewWebRTCServer(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capServer := WebRTCServer_ServerToClient(server)
		So(capServer.IsValid(), ShouldBeTrue)

		Convey("Write and Done lifecycle", func() {
			err := capServer.Write(ctx, func(params WebRTCServer_write_Params) error {
				return params.SetData([]byte("webrtc-data"))
			})
			So(err, ShouldBeNil)

			future, release := capServer.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status_ready)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "webrtc-data")

			_ = server.Close()
		})
	})
}
