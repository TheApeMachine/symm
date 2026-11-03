package broker

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestCheckEntryPhysicalRejectsNilPrice(t *testing.T) {
	Convey("entry is blocked when the book cannot be priced", t, func() {
		decision := CheckEntryPhysical(nil, "Q/USD")
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "no price book")
	})
}

func TestCheckEntryPhysicalRejectsThinBids(t *testing.T) {
	Convey("entry is blocked when top bids have no cash depth", t, func() {
		const symbol = "THIN/USD"

		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(symbol, kraken.InstrumentPair{
			Symbol:         symbol,
			Quote:          "USD",
			QtyPrecision:   8,
			PricePrecision: 2,
			CostPrecision:  2,
		})

		price := &Price{
			Instrument: instrument,
			Books:      NewBook(context.Background(), spot.NewNormalizer()),
		}
		decision := CheckEntryPhysical(price, symbol)
		So(decision.Allowed, ShouldBeFalse)
		So(decision.Reason, ShouldContainSubstring, "book too thin")
	})
}

