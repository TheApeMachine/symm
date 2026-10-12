package broker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

		Convey("GET /positions/lifecycle returns LifecycleReport", func() {
			req := httptest.NewRequest(http.MethodGet, "/positions/lifecycle", nil)
			resp, err := app.Test(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusOK)

			var report LifecycleReport
			So(json.NewDecoder(resp.Body).Decode(&report), ShouldBeNil)
			So(len(report.Lifecycles), ShouldEqual, 0)
			So(report.Performance.Closed, ShouldEqual, 0)
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

		Convey("Positions lifecycle reflects exit status", func() {
			qty := decimal.NewFromFloat64(0.2)
			So(desk.Enter("BTC/USD", qty), ShouldBeNil)

			reqOpen := httptest.NewRequest(http.MethodGet, "/positions", nil)
			respOpen, err := app.Test(reqOpen)
			So(err, ShouldBeNil)
			var openList []*Position
			So(json.NewDecoder(respOpen.Body).Decode(&openList), ShouldBeNil)
			So(len(openList), ShouldEqual, 1)

			reqLc := httptest.NewRequest(http.MethodGet, "/positions/lifecycle", nil)
			respLc, err := app.Test(reqLc)
			So(err, ShouldBeNil)
			var reportOpen LifecycleReport
			So(json.NewDecoder(respLc.Body).Decode(&reportOpen), ShouldBeNil)
			So(len(reportOpen.Lifecycles), ShouldEqual, 0)
			So(reportOpen.OpenCount, ShouldEqual, 1)

			desk.Apply(&kraken.Execution{
				Data: []kraken.ExecutionData{
					{
						Symbol:   "BTC/USD",
						Side:     "buy",
						ExecType: "trade",
						LastQty:  qty,
					},
					{
						Symbol:   "BTC/USD",
						Side:     "sell",
						ExecType: "trade",
						LastQty:  qty,
					},
				},
			})

			reqClosedPos := httptest.NewRequest(http.MethodGet, "/positions", nil)
			respClosedPos, err := app.Test(reqClosedPos)
			So(err, ShouldBeNil)
			var afterList []*Position
			So(json.NewDecoder(respClosedPos.Body).Decode(&afterList), ShouldBeNil)
			So(len(afterList), ShouldEqual, 0)

			reqClosedLc := httptest.NewRequest(http.MethodGet, "/positions/lifecycle", nil)
			respClosedLc, err := app.Test(reqClosedLc)
			So(err, ShouldBeNil)
			var reportClosed LifecycleReport
			So(json.NewDecoder(respClosedLc.Body).Decode(&reportClosed), ShouldBeNil)
			So(len(reportClosed.Lifecycles), ShouldEqual, 1)
			So(reportClosed.Lifecycles[0].Status, ShouldEqual, "closed")
			So(reportClosed.OpenCount, ShouldEqual, 0)
			So(reportClosed.Performance.Closed, ShouldEqual, 1)
		})
	})
}
