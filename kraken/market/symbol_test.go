package market

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolRowFromPrices(t *testing.T) {
	Convey("Given SymbolRowFromPrices", t, func() {
		eventAt := time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC)

		Convey("It should defer flat price windows", func() {
			_, err := SymbolRowFromPrices("BTC/USD", []float64{100, 100, 100}, 1000, 1, eventAt)

			So(errors.Is(err, ErrFlatPriceWindow), ShouldBeTrue)
		})

		Convey("It should derive value from microstructure when endpoints match", func() {
			row, err := SymbolRowFromPrices(
				"BTC/USD",
				[]float64{100, 100.001, 100.0005},
				1000,
				1,
				eventAt,
			)

			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Value, ShouldBeGreaterThan, 0)
		})

		Convey("It should build a row when prices move", func() {
			row, err := SymbolRowFromPrices("BTC/USD", []float64{100, 101}, 1000, 1, eventAt)

			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Value, ShouldBeGreaterThan, 0)
		})
	})
}
