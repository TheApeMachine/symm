package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStartPprof(t *testing.T) {
	Convey("Given the default HTTP mux used by the profiling server", t, func() {
		request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		response := httptest.NewRecorder()

		http.DefaultServeMux.ServeHTTP(response, request)

		Convey("It should expose the registered profiling index", func() {
			So(response.Code, ShouldEqual, http.StatusOK)
		})
	})
}
