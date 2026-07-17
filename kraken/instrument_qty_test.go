package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestInstrumentPairRoundQty floors onto the exchange lot grid.
*/
func TestInstrumentPairRoundQty(t *testing.T) {
	Convey("Given a BTC lot increment", t, func() {
		pair := InstrumentPair{
			QtyIncrement: 0.00000001,
			QtyPrecision: 8,
			CostMin:      *decimal.NewFromFloat64(5),
		}
		rounded := pair.RoundQty(decimal.NewFromFloat64(1.23456789111))

		So(rounded.Float64(), ShouldEqual, 1.23456789)
		So(pair.MeetsCostMin(decimal.NewFromFloat64(5)), ShouldBeTrue)
		So(pair.MeetsCostMin(decimal.NewFromFloat64(4.99)), ShouldBeFalse)
	})
}
