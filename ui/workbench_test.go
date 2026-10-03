package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorkbenchSupervisor(t *testing.T) {
	Convey("WorkbenchSupervisor", t, func() {
		Convey("healthURL derives health path accurately", func() {
			supervisor := NewWorkbenchSupervisor("http://127.0.0.1:8081/workbench/query")
			So(supervisor.healthURL(), ShouldEqual, "http://127.0.0.1:8081/health")

			remoteSupervisor := NewWorkbenchSupervisor("https://lakehouse.prod.internal:9000/workbench/query?token=secret")
			So(remoteSupervisor.healthURL(), ShouldEqual, "https://lakehouse.prod.internal:9000/health")
		})

		Convey("isLocal recognizes loopback variants", func() {
			So(NewWorkbenchSupervisor("http://127.0.0.1:8081/workbench/query").isLocal(), ShouldBeTrue)
			So(NewWorkbenchSupervisor("http://localhost:8081/workbench/query").isLocal(), ShouldBeTrue)
			So(NewWorkbenchSupervisor("http://0.0.0.0:8081/workbench/query").isLocal(), ShouldBeTrue)
			So(NewWorkbenchSupervisor("https://external-warehouse.com/workbench/query").isLocal(), ShouldBeFalse)
		})

		Convey("isHealthy reports false when server is absent", func() {
			supervisor := NewWorkbenchSupervisor("http://127.0.0.1:54321/workbench/query")
			So(supervisor.isHealthy(context.Background()), ShouldBeFalse)
		})

		Convey("isHealthy reports true when server responds OK", func() {
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"ok"}`))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer mockServer.Close()

			supervisor := NewWorkbenchSupervisor(mockServer.URL + "/workbench/query")
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			So(supervisor.isHealthy(ctx), ShouldBeTrue)
			So(supervisor.Ensure(ctx), ShouldBeNil)
		})
	})
}
