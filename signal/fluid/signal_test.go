package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var fluidCategories = []logic.CategoryType{
	logic.CategoryLaminar,
	logic.CategoryTurbulent,
	logic.CategoryInertial,
	logic.CategoryViscous,
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a fluid signal", t, func() {
		setFluidGridConfig()

		signal := NewSignal(context.Background())
		defer signal.Close()

		symbol := "ETH/EUR"
		start := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

		Convey("When ticker, snapshot, and stable update rows are measured", func() {
			So(measureTicker(signal, symbol, 1000, 100, 99.99, 100.01, start), ShouldBeNil)
			_, err := measureBook(signal, symbol, "snapshot", 5, 5, start.Add(10*time.Millisecond))
			So(err, ShouldBeNil)
			measurement, err := measureBook(
				signal,
				symbol,
				"update",
				5,
				5,
				start.Add(110*time.Millisecond),
			)

			Convey("It should classify a typed fluid measurement", func() {
				So(err, ShouldBeNil)
				So(measurement, ShouldNotBeNil)
				So(measurement.Source, ShouldEqual, logic.SourceFluid)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.Metric("laminar"), ShouldBeGreaterThan, 0)
				So(measurement.Metric("laminar"), ShouldBeGreaterThan, measurement.Metric("turbulent"))
				So(measurement.DominantCategory(), ShouldEqual, logic.CategoryLaminar)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
				So(measurement.EntryBaseline, ShouldBeGreaterThan, 0)
				So(measurement.ExitBaseline, ShouldBeGreaterThan, 0)
				So(measurement.EntryBaseline, ShouldBeGreaterThanOrEqualTo, measurement.ExitBaseline)
				So(measurement.Confidence, ShouldBeGreaterThan, measurement.EntryBaseline)
				So(distributionSum(measurement), ShouldAlmostEqual, 1, 0.0001)
				So(measurement.Metrics["vorticity"], ShouldNotBeNil)
			})
		})

		Convey("When book rows arrive before ticker volume is known", func() {
			So(measureTicker(signal, symbol, 0, 100, 99.99, 100.01, start), ShouldBeNil)
			_, err := measureBook(signal, symbol, "snapshot", 5, 5, start.Add(10*time.Millisecond))
			So(err, ShouldBeNil)
			measurement, err := measureBook(
				signal,
				symbol,
				"update",
				5,
				5,
				start.Add(110*time.Millisecond),
			)

			Convey("It should not emit an unready fluidflow measurement", func() {
				So(err, ShouldBeNil)
				So(measurement, ShouldBeNil)
			})
		})

		Convey("When trade flow lands before the next book reading", func() {
			So(measureTicker(signal, symbol, 1000, 100, 99.99, 100.01, start), ShouldBeNil)
			_, err := measureBook(signal, symbol, "snapshot", 5, 5, start.Add(10*time.Millisecond))
			So(err, ShouldBeNil)
			So(measureTrade(signal, symbol, 100.01, 1, "buy", start.Add(20*time.Millisecond)), ShouldBeNil)
			measurement, err := measureBook(
				signal,
				symbol,
				"update",
				5,
				4,
				start.Add(110*time.Millisecond),
			)

			Convey("It should carry trade evidence into mechanical metrics", func() {
				So(err, ShouldBeNil)
				So(measurement, ShouldNotBeNil)
				So(measurement.Metrics["vorticity"], ShouldNotBeNil)
				So(measurement.Metrics["viscosity"], ShouldNotBeNil)
				So(measurement.Metrics["reynolds"], ShouldNotBeNil)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
			})
		})

		Convey("When a ticker row has no timestamp", func() {
			_, err := signal.Measure(market.Input{
				Role: "ticker",
				Ticker: kraken.TickerDataSlice{{
					Symbol: symbol,
					Last:   100,
					Bid:    99.99,
					Ask:    100.01,
					Volume: 1000,
				}},
			}, nil)

			Convey("It should return the timestamp error", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "fluid: ticker event timestamp required")
			})
		})
	})
}

func measureTicker(
	signal *Signal,
	symbol string,
	volume float64,
	last float64,
	bid float64,
	ask float64,
	at time.Time,
) error {
	_, err := signal.Measure(market.Input{
		Role: "ticker",
		Ticker: kraken.TickerDataSlice{{
			Symbol:    symbol,
			Last:      last,
			Bid:       bid,
			Ask:       ask,
			Volume:    volume,
			Timestamp: at,
		}},
	}, nil)

	return err
}

func measureBook(
	signal *Signal,
	symbol string,
	frameType string,
	bidQty float64,
	askQty float64,
	at time.Time,
) (*logic.Measurement, error) {
	measurements, err := signal.Measure(market.Input{
		Role: "book",
		Book: kraken.BookDataSlice{{
			Symbol:    symbol,
			Type:      frameType,
			Timestamp: at,
			Bids: []kraken.BookLevel{
				{Price: 99.99, Qty: bidQty},
				{Price: 99.98, Qty: bidQty},
			},
			Asks: []kraken.BookLevel{
				{Price: 100.01, Qty: askQty},
				{Price: 100.02, Qty: askQty},
			},
		}},
	}, nil)

	if err != nil || len(measurements) == 0 {
		return nil, err
	}

	return measurements[0], nil
}

func measureTrade(
	signal *Signal,
	symbol string,
	price float64,
	qty float64,
	side string,
	at time.Time,
) error {
	_, err := signal.Measure(market.Input{
		Role: "trade",
		Trade: kraken.TradeDataSlice{{
			Symbol:    symbol,
			Side:      side,
			Price:     price,
			Qty:       qty,
			Timestamp: at,
		}},
	}, nil)

	return err
}

func distributionSum(measurement *logic.Measurement) float64 {
	total := 0.0

	for _, category := range fluidCategories {
		total += measurement.Distribution[category]
	}

	return total
}
