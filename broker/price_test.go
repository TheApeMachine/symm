package broker

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPriceWithFriction(t *testing.T) {
	Convey("Given a price stream with a known taker fee", t, func() {
		price := &Price{
			status:  types.READY,
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.0026"})

		Convey("When WithFriction is requested for unit quantity", func() {
			net, err := price.WithFriction("BTC/USD", 1)

			Convey("Then it returns the net value after round-trip fees", func() {
				So(err, ShouldBeNil)
				So(net.Float64(), ShouldAlmostEqual, 0.99479424, 1e-8)
			})
		})
	})
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := &Price{
		status:  types.READY,
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
	price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.0026"})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.WithFriction("BTC/USD", 1)
	}
}
