package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestService(t *testing.T) {
	Convey("Given a broker Service bound to a Desk and Balance", t, func() {
		balance := &Balance{Quote: "USD"}
		balance.measurement = data.NewMeasurement(
			0, "balance", "balance", 1, 1,
		).Write(
			data.NewMetric("equity", 1000, data.UnitCurrency, data.TimescaleInstantaneous),
			data.NewMetric("unrealized", 50, data.UnitCurrency, data.TimescaleInstantaneous),
			data.NewMetric("cash", 200, data.UnitCurrency, data.TimescaleInstantaneous),
		)
		desk := NewDesk(context.Background(), &dummyTransport{}, nil, balance)

		service := NewService(desk)

		app := fiber.New()
		service.Register(app)

		Convey("GET /positions returns positions array", func() {
			req := httptest.NewRequest(http.MethodGet, "/positions", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)
		})

		Convey("GET /balance returns unified BalanceReport", func() {
			req := httptest.NewRequest(http.MethodGet, "/balance", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)

			var report BalanceReport
			So(json.NewDecoder(resp.Body).Decode(&report), ShouldBeNil)
			So(report.Quote, ShouldEqual, "USD")
			So(report.Equity, ShouldEqual, "1000")
			So(report.Unrealized, ShouldEqual, "50")
		})

		Convey("GET /equity hits the same unified balance route", func() {
			req := httptest.NewRequest(http.MethodGet, "/equity", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)

			var report BalanceReport
			So(json.NewDecoder(resp.Body).Decode(&report), ShouldBeNil)
			So(report.Equity, ShouldEqual, "1000")
		})
	})
}
