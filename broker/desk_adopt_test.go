package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/config"
)

func TestDeskAdoptOpen(t *testing.T) {
	Convey("Given wallet holdings and a matching instrument pair", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.holdings["ETH/USD"] = &types.Holding{
			Symbol: "ETH/USD",
			Asset:  "ETH",
			Qty:    decimal.NewFromFloat64(3),
			Status: types.OPEN,
		}

		instrument := &Instrument{cache: map[string]kraken.InstrumentPair{}}
		instrument.Remember(kraken.InstrumentPair{
			Symbol: "ETH/USD",
			Base:   "ETH",
			Quote:  "USD",
			Status: "online",
		})

		desk := &Desk{
			balance:    balance,
			instrument: instrument,
			positions:  map[string]*Position{},
		}

		Convey("AdoptOpen creates a position shell for the existing lot", func() {
			desk.AdoptOpen()
			position, ok := desk.Position("ETH/USD")
			So(ok, ShouldBeTrue)
			So(position.Status(), ShouldEqual, types.OPEN)
			So(desk.HoldingCount(), ShouldEqual, 1)
		})
	})
}
