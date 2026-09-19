package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestHTTPServer(t *testing.T) {
	Convey("Given an HTTPServer node", t, func() {
		server := NewHTTPServer(types.Const(":0"))

		// Calling closure returns input as value
		out := server("seed-data")
		So(out, ShouldEqual, "seed-data")

		var (
			wsClients    sync.Map
			dataChannels sync.Map
			upgrader     websocket.Upgrader
		)
		handler := buildHTTPHandler(&wsClients, &dataChannels, &upgrader, webrtc.NewAPI(), webrtc.Configuration{})

		Convey("When requesting GET /workbench/signals", func() {
			req := httptest.NewRequest(http.MethodGet, "/workbench/signals", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			So(rec.Code, ShouldEqual, http.StatusOK)
			So(rec.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "*")
			So(rec.Body.Len(), ShouldBeGreaterThan, 0)
		})

		Convey("When requesting GET /workbench/primitives", func() {
			req := httptest.NewRequest(http.MethodGet, "/workbench/primitives", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			So(rec.Code, ShouldEqual, http.StatusOK)
			So(rec.Header().Get("Content-Type"), ShouldEqual, "application/json")
			So(rec.Body.Len(), ShouldBeGreaterThan, 0)
		})

		Convey("When requesting GET /hindsight/metric-map", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/metric-map", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			So(rec.Code, ShouldEqual, http.StatusOK)
			So(rec.Header().Get("Content-Type"), ShouldEqual, "application/json")
		})

		Convey("When requesting OPTIONS /workbench/primitives", func() {
			req := httptest.NewRequest(http.MethodOptions, "/workbench/primitives", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			So(rec.Code, ShouldEqual, http.StatusOK)
			So(rec.Header().Get("Access-Control-Allow-Origin"), ShouldEqual, "*")
		})

		Convey("When posting to /workbench/query", func() {
			req := httptest.NewRequest(http.MethodPost, "/workbench/query", bytes.NewReader([]byte(`{"sql":"SELECT 1"}`)))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			So(rec.Code, ShouldEqual, http.StatusOK)
		})
	})
}
