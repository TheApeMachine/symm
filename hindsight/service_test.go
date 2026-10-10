package hindsight

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func TestService(t *testing.T) {
	Convey("Given a hindsight Service with nil store", t, func() {
		ctx := context.Background()
		service := NewService(ctx, nil)

		app := fiber.New()
		service.Register(app)

		Convey("Nil safety on Register", func() {
			var nilService *Service
			nilService.Register(app)
			service.Register(nil)
		})

		Convey("GET /hindsight/metric-map returns signal semantics", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/metric-map", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("GET /hindsight/runs reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/runs", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/symbols reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/symbols", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/excursions reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/excursions", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/data reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/data", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/captures reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/captures", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/envelope reports service unavailable when store is nil", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/envelope?seq=1", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
		})

		Convey("GET /hindsight/gaps returns empty list", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/gaps", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("GET /hindsight/lifecycle returns empty list", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/lifecycle", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("GET /hindsight/states returns empty list", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/states", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("GET /hindsight/state returns not found", func() {
			req := httptest.NewRequest(http.MethodGet, "/hindsight/state", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusNotFound)
		})
	})
}
