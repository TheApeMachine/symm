package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestEntryTrailCoversRoundTripFeeAndHalfSpread(t *testing.T) {
	Convey("Given a filled lot with fee and a warm touch book", t, func() {
		price := NewPrice(nil)
		So(price.RememberFee("XCN/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"XCN/USD","last":"1.00","bid":"0.995","ask":"1.005"}]}`,
		))

		position := &Position{price: price}
		holding := &types.Holding{
			Symbol:     "XCN/USD",
			EntryPrice: decimal.NewFromFloat64(1.0),
			EntryFee:   decimal.NewFromFloat64(0.0026),
			Qty:        decimal.NewFromFloat64(1),
		}

		trail := position.EntryTrail(holding)

		Convey("Then the bind width exceeds fee-only (~0.26%) survival", func() {
			So(trail, ShouldBeGreaterThan, 0.0026)
			// 2*0.0026 + half of (0.01/1.0) = 0.0052 + 0.005 = 0.0102
			So(trail, ShouldBeGreaterThanOrEqualTo, 0.01)
		})
	})
}
