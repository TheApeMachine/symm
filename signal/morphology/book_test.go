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

func TestProjectShape(t *testing.T) {
	Convey("Given a crossed or degenerate book", t, func() {
		orderBook := testBook()
		orderBook.NoBookCrossing = false
		addLevel(orderBook, book.Bid, 101, 1, time.Now())
		addLevel(orderBook, book.Ask, 99, 1, time.Now())

		_, _, _, ok := projectShape(orderBook)

		Convey("projectShape reports not-ok, never fabricating a shape", func() {
			So(ok, ShouldBeFalse)
		})
	})

	Convey("Given a single-level symmetric book", t, func() {
		orderBook := testBook()
		now := time.Now()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		bidFolded, askFolded, whole, ok := projectShape(orderBook)

		Convey("bilateral shapes are folded onto the positive distance axis", func() {
			So(ok, ShouldBeTrue)
			// Bid touch (99, mid 100, spread 2) folds to (100-99)/2 = +0.5,
			// and the ask touch folds to (101-100)/2 = +0.5: one mirrored book.
			So(bidFolded[0].Position, ShouldAlmostEqual, 0.5)
			So(askFolded[0].Position, ShouldAlmostEqual, 0.5)
		})

		Convey("the whole-book shape retains signed positions", func() {
			So(ok, ShouldBeTrue)
			// whole book has two points: bid at -0.5 and ask at +0.5.
			So(len(whole), ShouldEqual, 2)
			So(whole[0].Position, ShouldAlmostEqual, -0.5)
			So(whole[1].Position, ShouldAlmostEqual, 0.5)
		})
	})
}

func TestBookStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	later := time.Unix(1_700_000_001, 0)

	Convey("Given an exactly mirrored multi-level book", t, func() {
		orderBook := testBook()
		// A mirrored book by notional: each level carries the same notional C
		// (quantity = C/price), so the bid side's mass profile is identical to
		// the ask side's when reflected — identical folded positions with
		// identical mass at each.
		const commonNotional = 100000.0
		addLevel(orderBook, book.Bid, 98, commonNotional/98, now)
		addLevel(orderBook, book.Bid, 99, commonNotional/99, now)
		addLevel(orderBook, book.Ask, 101, commonNotional/101, now)
		addLevel(orderBook, book.Ask, 102, commonNotional/102, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("bilateral distance and KS are exactly zero", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["book_shape_distance"].Raw, ShouldAlmostEqual, 0)
			So(measurement.Metrics["book_shape_ks"].Raw, ShouldAlmostEqual, 0)
		})
	})

	Convey("Given an asymmetric book", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Bid, 97, 1, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("bilateral distance and KS are positive", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Metrics["book_shape_distance"].Raw, ShouldBeGreaterThan, 0)
			So(measurement.Metrics["book_shape_ks"].Raw, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given scale-equivalent books", t, func() {
		Convey("rescaling all quantities yields equivalent normalized morphology", func() {
			first := testBook()
			addLevel(first, book.Bid, 98, 1, now)
			addLevel(first, book.Bid, 99, 2, now)
			addLevel(first, book.Ask, 101, 2, now)
			addLevel(first, book.Ask, 102, 1, now)

			second := testBook()
			addLevel(second, book.Bid, 98, 10, now)
			addLevel(second, book.Bid, 99, 20, now)
			addLevel(second, book.Ask, 101, 20, now)
			addLevel(second, book.Ask, 102, 10, now)

			workspace1 := runtime.NewWorkspace(nil)
			workspace1.Share("book", first, "BTC/USD")
			workspace2 := runtime.NewWorkspace(nil)
			workspace2.Share("book", second, "ETH/USD")

			firstMeasurement := NewBook(workspace1).Step("BTC/USD", now)
			secondMeasurement := NewBook(workspace2).Step("ETH/USD", now)

			So(firstMeasurement, ShouldNotBeNil)
			So(secondMeasurement, ShouldNotBeNil)

			So(firstMeasurement.Metrics["book_shape_distance"].Raw, ShouldAlmostEqual, secondMeasurement.Metrics["book_shape_distance"].Raw)
			So(firstMeasurement.Metrics["book_shape_ks"].Raw, ShouldAlmostEqual, secondMeasurement.Metrics["book_shape_ks"].Raw)
			So(firstMeasurement.Metrics["concentration:bid"].Raw, ShouldAlmostEqual, secondMeasurement.Metrics["concentration:bid"].Raw)
			So(firstMeasurement.Metrics["entropy:bid"].Raw, ShouldAlmostEqual, secondMeasurement.Metrics["entropy:bid"].Raw)
		})
	})

	Convey("Given the first observation of a symbol", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("structural change is undefined, never fabricated as zero", func() {
			measurement := entity.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
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

/*
BenchmarkStep measures the steady-state cost and allocation count of one
morphology Step against a realistic ten-level book, once warm.
*/
func BenchmarkStep(benchmark *testing.B) {
	now := time.Unix(1_700_000_000, 0)

	orderBook := testBook()

	for level := 0; level < 10; level++ {
		addLevel(orderBook, book.Bid, 99-float64(level), 2, now.Add(time.Duration(level)*time.Second))
		addLevel(orderBook, book.Ask, 101+float64(level), 2, now.Add(time.Duration(level)*time.Second))
	}

	workspace := runtime.NewWorkspace(nil)
	workspace.Share("book", orderBook, "BTC/USD")

	entity := NewBook(workspace)

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for index := 0; index < benchmark.N; index++ {
		_ = entity.Step("BTC/USD", now)
	}
}

/*
TestReadBookScope proves the book is consumed entirely inside the protected
read callback: readBook takes a callback and returns no value, so the caller
can only observe floats copied out during the locked scope, and the callback
runs to completion synchronously (a value set inside it is visible immediately
after readBook returns). There is no path that returns or retains *book.Book.
*/
func TestReadBookScope(t *testing.T) {
	Convey("Given a shared book", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, time.Now())
		addLevel(orderBook, book.Ask, 101, 2, time.Now())

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		entity := NewBook(workspace)

		Convey("readBook runs its callback synchronously and returns nothing", func() {
			completed := false

			entity.readBook("BTC/USD", func(sharedBook *book.Book) {
				completed = sharedBook != nil
			})

			// The callback has fully run by the time readBook returns: the
			// book was consumed inside the locked scope, never handed out.
			So(completed, ShouldBeTrue)
		})
	})
}
