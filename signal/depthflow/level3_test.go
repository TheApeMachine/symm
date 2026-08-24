package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/theapemachine/symm/nomagique/runtime"
)

func testBook() *book.Book {
	return book.New()
}

func addLevel(
	orderBook *book.Book,
	direction book.BookDirection,
	price float64,
	quantity float64,
	at time.Time,
) {
	orderBook.Update(&book.UpdateOptions{
		Direction: direction,
		Price:     decimal.NewFromFloat64(price),
		Quantity:  decimal.NewFromFloat64(quantity),
		Timestamp: at,
	})
}

func TestLevel3Step(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)

	Convey("Given a valid shared book", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewLevel3(workspace)

		Convey("Step produces exactly one measurement with no warmup", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			So(measurement.Metrics["book_notional:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(measurement.Metrics["book_notional:ask"].Raw, ShouldAlmostEqual, 202.0, 1e-12)
			So(measurement.Metrics["book_notional"].Raw, ShouldAlmostEqual, 400.0, 1e-12)
			So(measurement.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["touch_imbalance"].Raw, ShouldAlmostEqual, -0.01, 1e-12)
			So(measurement.Metrics["imbalance_resolution_gap"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
			So(measurement.Metrics["imbalance_resolution_distance"].Raw, ShouldAlmostEqual, 0.0, 1e-12)

			// First observation: support is one sample, so maturity is zero and
			// the flow rate metrics are absent (no previous interval exists).
			So(measurement.Maturity, ShouldEqual, 0.0)
			So(measurement.Metrics, ShouldNotContainKey, "net_displayed_flow_rate:bid")
			So(measurement.Metrics, ShouldNotContainKey, "book_turnover_rate")
		})

		Convey("stateful metrics advance over a multi-update sequence", func() {
			first := entity.Step("BTC/USD", now)
			So(first.Err, ShouldBeNil)

			addLevel(orderBook, book.Bid, 99, 4, later)

			second := entity.Step("BTC/USD", later)

			So(second, ShouldNotBeNil)
			So(second.Err, ShouldBeNil)

			So(second.Metrics["book_notional:bid"].Raw, ShouldAlmostEqual, 396.0, 1e-12)
			So(second.Metrics["book_imbalance"].Raw, ShouldAlmostEqual, 194.0/598.0, 1e-12)
			So(second.Metrics["net_displayed_flow:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(second.Metrics["added_notional:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(second.Metrics["removed_notional:bid"].Raw, ShouldAlmostEqual, 0.0, 1e-12)
			So(second.Metrics["net_displayed_flow_rate:bid"].Raw, ShouldAlmostEqual, 198.0, 1e-12)
			So(second.Metrics["flow_activity_imbalance"].Raw, ShouldAlmostEqual, 1.0, 1e-12)

			// Two retained samples raise the estimator maturity and the noise
			// model now produces a positive SNR.
			So(second.Maturity, ShouldEqual, 0.5)
			So(second.SNR, ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a crossed shared book", t, func() {
		orderBook := testBook()
		orderBook.NoBookCrossing = false
		addLevel(orderBook, book.Bid, 101, 1, now)
		addLevel(orderBook, book.Ask, 99, 1, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewLevel3(workspace)

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
