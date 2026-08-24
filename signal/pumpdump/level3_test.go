package pumpdump

import (
	"testing"
	"time"

	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func populatedBook(bidPrice float64, bidQty float64, askPrice float64, askQty float64) *book.Book {
	resident := book.New()
	resident.Update(&book.UpdateOptions{
		Direction: book.Bid,
		Price:     decimal.NewFromFloat64(bidPrice),
		Quantity:  decimal.NewFromFloat64(bidQty),
		Timestamp: time.Unix(1_700_000_000, 0),
	})
	resident.Update(&book.UpdateOptions{
		Direction: book.Ask,
		Price:     decimal.NewFromFloat64(askPrice),
		Quantity:  decimal.NewFromFloat64(askQty),
		Timestamp: time.Unix(1_700_000_000, 0),
	})

	return resident
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a shared book with an executable touch", t, func() {
		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", populatedBook(99, 1, 101, 1), "BTC/USD")
		entity := NewLevel3(workspace)

		Convey("Step reads the shared book and projects the touch", func() {
			measurement := entity.Step("BTC/USD", time.Unix(1_700_000_000, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			// A stateless direct measurement is whole.
			So(measurement.Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a symbol with no shared book", t, func() {
		entity := NewLevel3(runtime.NewWorkspace(nil))

		Convey("Step returns a descriptive measurement error for the missing book", func() {
			measurement := entity.Step("MISSING", time.Unix(1_700_000_000, 0))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}
