package controlflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestMatchServer(t *testing.T) {
	Convey("Given a MatchServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewMatch(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		capMatch := Match_ServerToClient(server)
		So(capMatch.IsValid(), ShouldBeTrue)

		Convey("Setting pattern and checking non-matching payload", func() {
			err := capMatch.Write(ctx, func(params Match_write_Params) error {
				if err := params.SetPattern("instrument"); err != nil {
					return err
				}
				return params.SetData([]byte(`{"channel":"heartbeat"}`))
			})
			So(err, ShouldBeNil)

			future, release := capMatch.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Matched(), ShouldBeFalse)
			So(results.HasOut(), ShouldBeFalse)
		})

		Convey("Setting pattern and checking matching payload", func() {
			instrumentPayload := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":[]}}`)
			err := capMatch.Write(ctx, func(params Match_write_Params) error {
				if err := params.SetPattern("instrument"); err != nil {
					return err
				}
				return params.SetData(instrumentPayload)
			})
			So(err, ShouldBeNil)

			future, release := capMatch.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Matched(), ShouldBeTrue)
			So(results.HasOut(), ShouldBeTrue)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, string(instrumentPayload))
		})
	})
}
