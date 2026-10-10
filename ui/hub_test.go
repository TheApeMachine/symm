package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHub(t *testing.T) {
	Convey("Given a ui Hub", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx)

		Convey("Hub provides a Fiber App with registered routes", func() {
			app := hub.App()
			So(app, ShouldNotBeNil)

			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			// Non-websocket request to /ws should require upgrade
			So(resp.StatusCode, ShouldEqual, http.StatusUpgradeRequired)
		})
	})
}
