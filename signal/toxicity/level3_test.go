package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func bookFixture(bid float64, bidQty float64, ask float64, askQty float64) *book.Book {
	currentBook := book.New()
	currentBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		Price:     decimal.NewFromFloat64(bid),
		Quantity:  decimal.NewFromFloat64(bidQty),
		Timestamp: time.Unix(1_700_000_000, 0),
	})
	currentBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		Price:     decimal.NewFromFloat64(ask),
		Quantity:  decimal.NewFromFloat64(askQty),
		Timestamp: time.Unix(1_700_000_000, 0),
	})

	return currentBook
}

func crossedBookFixture() *book.Book {
	currentBook := book.New()
	currentBook.NoBookCrossing = false
	currentBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		Price:     decimal.NewFromFloat64(101),
		Quantity:  decimal.NewFromFloat64(10),
		Timestamp: time.Unix(1_700_000_000, 0),
	})
	currentBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		Price:     decimal.NewFromFloat64(99),
		Quantity:  decimal.NewFromFloat64(12),
		Timestamp: time.Unix(1_700_000_000, 0),
	})

	return currentBook
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a shared book", t, func() {
		workspace := runtime.NewWorkspace(nil)
		entity := NewLevel3(workspace)
		workspace.Share("book", bookFixture(99, 10, 101, 12), "BTC/USD")

		Convey("the first observation anchors the previous touch", func() {
			measurement := entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["touch_quantity:ask"].Raw, ShouldEqual, 12.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["unfilled_residual_quantity:bid"].Raw, ShouldEqual, 10.0)

			// Stateless direct measurement is whole (Maturity 1).
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes a bid retreat", func() {
			entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))
			workspace.Share("book", bookFixture(98, 5, 101, 12), "BTC/USD")

			measurement := entity.Step("BTC/USD", time.Unix(1_700_000_001, 0))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["previous_best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 98.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldAlmostEqual, math.Log(98.0/99.0), 1e-12)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes an unchanged-touch withdrawal", func() {
			entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))
			workspace.Share("book", bookFixture(99, 4, 101, 12), "BTC/USD")

			measurement := entity.Step("BTC/USD", time.Unix(1_700_000_001, 0))

			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 0.0)
		})
	})

	Convey("Given a crossed shared book", t, func() {
		workspace := runtime.NewWorkspace(nil)
		entity := NewLevel3(workspace)
		workspace.Share("book", crossedBookFixture(), "BTC/USD")

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})

	Convey("Given a missing shared book", t, func() {
		workspace := runtime.NewWorkspace(nil)
		entity := NewLevel3(workspace)

		Convey("Step panics rather than silently skipping", func() {
			So(func() {
				entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))
			}, ShouldPanic)
		})
	})
}
