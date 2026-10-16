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
		service := NewService(context.Background(), nil)

		app := fiber.New()
		service.Register(app)

		Convey("Nil safety on Register", func() {
			var nilService *Service
			nilService.Register(app)
			service.Register(nil)
		})

		Convey("GET /hindsight/metric-map is explicitly not implemented", func() {
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/hindsight/metric-map", nil))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusNotImplemented)
		})

		Convey("Every store-backed route is unavailable, never an empty success", func() {
			for _, route := range []string{
				"/hindsight/runs",
				"/hindsight/symbols?run=1",
				"/hindsight/excursions?run=1&symbol=BTC/USD",
				"/hindsight/data?run=1",
				"/hindsight/captures?run=1",
				"/hindsight/envelope?run=1&seq=1",
				"/hindsight/gaps?run=1",
				"/hindsight/lifecycle?run=1&symbol=BTC/USD",
				"/hindsight/states?run=1",
				"/hindsight/state?run=1&seq=1",
				"/training/fragments?run=1",
				"/training/fragments/1/points",
			} {
				resp, err := app.Test(httptest.NewRequest(http.MethodGet, route, nil))
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusServiceUnavailable)
			}
		})
	})

	Convey("queryUint rejects what it cannot parse", t, func() {
		_, _, err := queryUint("", "run", true)
		So(err, ShouldNotBeNil)

		_, present, err := queryUint("", "limit", false)
		So(err, ShouldBeNil)
		So(present, ShouldBeFalse)

		_, _, err = queryUint("abc", "limit", false)
		So(err, ShouldNotBeNil)

		value, present, err := queryUint("42", "limit", false)
		So(err, ShouldBeNil)
		So(present, ShouldBeTrue)
		So(value, ShouldEqual, uint64(42))
	})
}
