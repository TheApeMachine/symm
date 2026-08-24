package depthflow

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/book"

	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestNewSignal(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	Convey("Given a workspace with a shared book", t, func() {
		orderBook := testBook()
		addLevel(orderBook, book.Bid, 99, 2, now)
		addLevel(orderBook, book.Ask, 101, 2, now)

		workspace := runtime.NewWorkspace(nil)
		workspace.Share("book", orderBook, "BTC/USD")

		signal := NewSignal(context.Background(), workspace)

		Convey("Name reports the signal identity", func() {
			So(signal.Name(), ShouldEqual, "depthflow")
		})

		Convey("Error is nil on a healthy signal", func() {
			So(signal.Error(), ShouldBeNil)
		})

		Convey("Step delegates to the Level3 entity", func() {
			measurement := signal.Step("BTC/USD", now)

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
		})

		Convey("Close releases the signal", func() {
			So(signal.Close(), ShouldBeNil)
		})
	})
}
