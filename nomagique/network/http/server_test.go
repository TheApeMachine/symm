package http

import (
	"context"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestHTTPServer(t *testing.T) {
	Convey("Given an HTTPServerServer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		server := NewHTTPServer(ctx)
		So(server, ShouldNotBeNil)
		So(server.Status(), ShouldEqual, runtime.WAITING)

		capServer := HTTPServer_ServerToClient(server)
		So(capServer.IsValid(), ShouldBeTrue)
		defer capServer.Release()
		defer func() { So(server.Close(), ShouldBeNil) }()
		So(capServer.Write(ctx, func(params HTTPServer_write_Params) error { return params.SetAddress("127.0.0.1:0") }), ShouldBeNil)
		So(capServer.WaitStreaming(), ShouldBeNil)
		So(server.Status(), ShouldEqual, runtime.READY)

		Convey("HTTP Handler endpoints", func() {
			handler := server.Handler()

			// Test GET /workbench/primitives
			req := httptest.NewRequest("GET", "/workbench/primitives", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			So(rec.Code, ShouldEqual, 200)

			// Test GET /workbench/signals
			req = httptest.NewRequest("GET", "/workbench/signals", nil)
			rec = httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			So(rec.Code, ShouldEqual, 200)

			// Test OPTIONS CORS
			req = httptest.NewRequest("OPTIONS", "/workbench/primitives", nil)
			rec = httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			So(rec.Code, ShouldEqual, 200)
			So(rec.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "*")
		})

		Convey("Write and Done lifecycle", func() {
			err := capServer.Write(ctx, func(params HTTPServer_write_Params) error {
				return params.SetData([]byte("http-data"))
			})
			So(err, ShouldBeNil)

			future, release := capServer.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)
			So(results.Status(), ShouldEqual, runtime.Status_ready)

			out, err := results.Out()
			So(err, ShouldBeNil)
			So(string(out), ShouldEqual, "http-data")

			So(server.Close(), ShouldBeNil)
		})

		Convey("When another server finds the port already held", func() {
			other := HTTPServer_ServerToClient(NewHTTPServer(ctx))
			defer other.Release()
			So(other.Write(ctx, func(params HTTPServer_write_Params) error { return params.SetAddress(server.address) }), ShouldBeNil)

			future, release := other.Done(ctx, nil)
			defer release()

			_, err := future.Struct()

			Convey("Then it fails instead of serving nobody", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "failed to listen")
			})
		})
	})
}
