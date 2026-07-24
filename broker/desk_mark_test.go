package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestDeskOnTickerPublishesMark(t *testing.T) {
	Convey("Given an open lot already marked once", t, func() {
		ui := make(chan []byte, 2)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		holding := &types.Holding{
			Symbol:     "ESPORTS/USD",
			Asset:      "ESPORTS",
			Qty:        decimal.NewFromFloat64(100),
			EntryPrice: decimal.NewFromFloat64(1),
			EntryFee:   decimal.NewFromFloat64(0),
			Status:     types.OPEN,
		}
		balance.holdings["ESPORTS/USD"] = holding

		pair := kraken.InstrumentPair{
			Symbol: "ESPORTS/USD",
			Base:   "ESPORTS",
			Quote:  "USD",
			Status: "online",
		}
		price := NewPrice(nil)
		So(price.RememberFee("ESPORTS/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck(&kraken.Ticker{Data: []kraken.TickerData{{
			Symbol: "ESPORTS/USD",
			Bid:    decimal.NewFromFloat64(1.0),
			Ask:    decimal.NewFromFloat64(1.01),
			Last:   decimal.NewFromFloat64(1.0),
		}}})

		position := NewPosition(nil, nil, price, balance, pair)
		So(position.setStatus(types.OPEN), ShouldBeNil)

		desk := &Desk{
			balance:   balance,
			price:     price,
			positions: map[string]*Position{"ESPORTS/USD": position},
		}

		Convey("When a newer ticker arrives for that lot", func() {
			desk.onTicker(&kraken.Ticker{
				Channel: "ticker",
				Type:    "update",
				Data: []kraken.TickerData{{
					Symbol: "ESPORTS/USD",
					Bid:    decimal.NewFromFloat64(0.9),
					Ask:    decimal.NewFromFloat64(0.91),
					Last:   decimal.NewFromFloat64(0.9),
				}},
			})

			Convey("It republishes holdings so the UI sees the new mark", func() {
				So(holding.Mark, ShouldNotBeNil)
				So(holding.Mark.Float64(), ShouldAlmostEqual, 0.9, 1e-9)
				So(holding.PnL, ShouldNotBeNil)
				So(len(ui), ShouldEqual, 1)
			})
		})
	})
}
