package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
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

			// When workbench is running, /workbench/query proxies the query
			body := strings.NewReader(`{"sql":"SHOW ALL TABLES"}`)
			queryReq := httptest.NewRequest(http.MethodPost, "/workbench/query", body)
			queryReq.Header.Set("Content-Type", "application/json")
			queryResp, err := app.Test(queryReq)
			So(err, ShouldBeNil)
			So(queryResp.StatusCode, ShouldEqual, http.StatusOK)
			So(queryResp.Header.Get("Content-Type"), ShouldEqual, "application/vnd.apache.arrow.stream")

			// When workbench target is down, /workbench/query returns 503
			oldURL := system.Cfg.Workbench.URL
			system.Cfg.Workbench.URL = "http://127.0.0.1:59999/workbench/query"
			badReq := httptest.NewRequest(http.MethodPost, "/workbench/query", strings.NewReader(`{"sql":"SELECT 1"}`))
			badReq.Header.Set("Content-Type", "application/json")
			badResp, err := app.Test(badReq)
			So(err, ShouldBeNil)
			So(badResp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
			system.Cfg.Workbench.URL = oldURL
		})
	})
}
