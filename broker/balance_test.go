package broker

import (
	"testing"
	"unsafe"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

type testTee struct {
	pushed []*data.Measurement
}

func (tee *testTee) Push(measurement *data.Measurement) {
	tee.pushed = append(tee.pushed, measurement)
}

func (tee *testTee) Next() unsafe.Pointer {
	return nil
}

func (tee *testTee) Close() error {
	return nil
}

func TestBalance(t *testing.T) {
	Convey("Given a Balance instance with a UI Tee", t, func() {
		tee := &testTee{}
		balance := &Balance{
			Quote: "USD",
			uiTee: tee,
			wallet: &kraken.Balance{
				Data: []kraken.BalanceData{
					{
						Asset:     "USD",
						Balance:   decimal.NewFromFloat64(1000),
						Available: decimal.NewFromFloat64(1000),
					},
				},
			},
		}

		Convey("When a measurement is constructed from balance state", func() {
			metrics := []*data.Metric{
				data.NewMetric("unrealized", 50, data.UnitCurrency, data.TimescaleInstantaneous),
				data.NewMetric("equity", 1050, data.UnitCurrency, data.TimescaleInstantaneous),
			}

			for _, row := range balance.wallet.Data {
				metrics = append(metrics, data.NewMetric(
					"cash",
					row.Available.Float64(),
					data.UnitCurrency,
					data.TimescaleInstantaneous,
				))
			}

			measurement := data.NewMeasurement(
				0, "balance", "balance", 1, 1,
			).Write(metrics...)

			balance.measurement = measurement
			balance.uiTee.Push(balance.measurement)

			So(len(tee.pushed), ShouldEqual, 1)
			pushed := tee.pushed[0]
			So(pushed.Source, ShouldEqual, "balance")

			var cash, equity, unrealized float64
			for entry := range pushed.Read("cash") {
				if entry.Metric != nil {
					cash = entry.Metric.Raw
				}
			}
			for entry := range pushed.Read("equity") {
				if entry.Metric != nil {
					equity = entry.Metric.Raw
				}
			}
			for entry := range pushed.Read("unrealized") {
				if entry.Metric != nil {
					unrealized = entry.Metric.Raw
				}
			}

			So(cash, ShouldEqual, 1000)
			So(equity, ShouldEqual, 1050)
			So(unrealized, ShouldEqual, 50)
		})
	})
}
