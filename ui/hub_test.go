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

			mockWorkbench := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/vnd.apache.arrow.stream")
				writer.WriteHeader(http.StatusOK)
			}))
			defer mockWorkbench.Close()

			oldURL := system.Cfg.Workbench.URL
			system.Cfg.Workbench.URL = mockWorkbench.URL
			defer func() { system.Cfg.Workbench.URL = oldURL }()

			// When workbench is running, /workbench/query proxies the query
			body := strings.NewReader(`{"sql":"SHOW ALL TABLES"}`)
			queryReq := httptest.NewRequest(http.MethodPost, "/workbench/query", body)
			queryReq.Header.Set("Content-Type", "application/json")
			queryReq.Header.Set("Origin", "http://localhost:3000")
			queryResp, err := app.Test(queryReq)
			So(err, ShouldBeNil)
			So(queryResp.StatusCode, ShouldEqual, http.StatusOK)
			So(queryResp.Header.Get("Content-Type"), ShouldEqual, "application/vnd.apache.arrow.stream")
			So(queryResp.Header.Get("Access-Control-Allow-Origin"), ShouldEqual, "http://localhost:3000")

			// When workbench target is down, /workbench/query returns 503
			system.Cfg.Workbench.URL = "http://127.0.0.1:59999/workbench/query"
			badReq := httptest.NewRequest(http.MethodPost, "/workbench/query", strings.NewReader(`{"sql":"SELECT 1"}`))
			badReq.Header.Set("Content-Type", "application/json")
			badResp, err := app.Test(badReq)
			So(err, ShouldBeNil)
			So(badResp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})
	})
}
