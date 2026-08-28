package morphology

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

func addLevel(orderBook *book.Book, direction book.BookDirection, price, quantity float64, at time.Time) {
	orderBook.Update(&book.UpdateOptions{
		Direction: direction,
		Price:     decimal.NewFromFloat64(price),
		Quantity:  decimal.NewFromFloat64(quantity),
		Timestamp: at,
	})
}

func TestShapeCoordinates(t *testing.T) {
	Convey("Given a crossed or degenerate book", t, func() {
		orderBook := testBook()
		orderBook.NoBookCrossing = false
		addLevel(orderBook, book.Bid, 101, 1, time.Now())
		addLevel(orderBook, book.Ask, 99, 1, time.Now())

		_, _, _, _, ok := shapeCoordinates(orderBook)

		Convey("shapeCoordinates reports not-ok, never fabricating a shape", func() {
			So(ok, ShouldBeFalse)
		})
	})

	Convey("Given a single-level symmetric book", t, func() {
		orderBook := testBook()
		now := time.Now()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		bidPositions, bidWeights, askPositions, askWeights, ok := shapeCoordinates(orderBook)

		Convey("positions are spread-normalized around the midpoint", func() {
			So(ok, ShouldBeTrue)
			So(bidPositions[0], ShouldAlmostEqual, -0.5)
			So(askPositions[0], ShouldAlmostEqual, 0.5)
		})

		Convey("weights are side notional (price × quantity)", func() {
			So(bidWeights[0], ShouldAlmostEqual, 198.0)
			So(askWeights[0], ShouldAlmostEqual, 202.0)
		})
	})
}

func TestBookStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)

	Convey("Given a valid shared book", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("Step emits the dimensionless shape facts, never a judge's score", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)

			// Single-level symmetric book: the bid mass sits entirely at −0.5
			// and the ask mass at +0.5, so the shapes are maximally separated.
			So(measurement.Metrics["book_shape_distance"].Raw, ShouldAlmostEqual, 1.0)
			So(measurement.Metrics["book_shape_ks"].Raw, ShouldAlmostEqual, 1.0)

			// Each side monopolizes a single level: full concentration, zero
			// entropy.
			So(measurement.Metrics["concentration:bid"].Raw, ShouldAlmostEqual, 1.0)
			So(measurement.Metrics["concentration:ask"].Raw, ShouldAlmostEqual, 1.0)
			So(measurement.Metrics["entropy:bid"].Raw, ShouldAlmostEqual, 0.0)
			So(measurement.Metrics["entropy:ask"].Raw, ShouldAlmostEqual, 0.0)

			// First observation has no prior shape, so structural change is
			// honestly absent rather than fabricated.
			So(measurement.Metrics, ShouldNotContainKey, "morphology_change")
		})

		Convey("structural change appears once a prior shape exists", func() {
			first := entity.Step("BTC/USD", now)
			So(first.Err, ShouldBeNil)

			addLevel(orderBook, book.Bid, 98, 4, later)

			second := entity.Step("BTC/USD", later)

			So(second, ShouldNotBeNil)
			So(second.Err, ShouldBeNil)
			So(second.Metrics, ShouldContainKey, "morphology_change")
			So(second.Metrics["morphology_change"].Raw, ShouldBeGreaterThan, 0.0)
		})
	})

	Convey("Given a crossed shared book", t, func() {
		orderBook := testBook()
		orderBook.NoBookCrossing = false
		addLevel(orderBook, book.Bid, 101, 1, now)
		addLevel(orderBook, book.Ask, 99, 1, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("Step returns nil for a book with no shape", func() {
			So(entity.Step("BTC/USD", now), ShouldBeNil)
		})
	})
}
