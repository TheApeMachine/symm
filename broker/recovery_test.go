package broker

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

func TestRecoveryRecoveredPosition(t *testing.T) {
	Convey("Given a stoploss loaded before the first live ticker", t, func() {
		price, api := newPriceSurface(t, "SIM/USD")
		positions := &sync.Map{}
		pair := kraken.InstrumentPair{
			Symbol: "SIM/USD",
			Base:   "SIM",
			Quote:  "USD",
		}
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(pair.Symbol, pair)
		recovery := &Recovery{
			ctx:        t.Context(),
			api:        api,
			instrument: instrument,
			price:      price,
			positions:  positions,
		}
		stoploss := newBrokerStoploss(t)

		Convey("It should attach that stop without reading the ticker cache", func() {
			position := recovery.recoveredPosition(
				pair,
				"SIM",
				decimal.NewFromFloat64(2),
				decimal.NewFromFloat64(100.02),
				decimal.NewFromFloat64(0.20),
				time.Now().UTC(),
				stoploss,
			)

			So(position.Holding.Stoploss, ShouldEqual, stoploss)
			So(position.Holding.Mark.Cmp(stoploss.Mark), ShouldEqual, 0)
			_, found := positions.Load(pair.Symbol)
			So(found, ShouldBeTrue)
		})
	})
}
