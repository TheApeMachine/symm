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

func TestDeskSeedEconomicsFromHistory(t *testing.T) {
	Convey("Given an adopted ESPORTS lot without entry and indexed buy fills", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 4), config.Fixture().Market)
		balance.quote = "USD"
		holding := &types.Holding{
			Symbol: "ESPORTS/USD",
			Asset:  "ESPORTS",
			Qty:    decimal.NewFromFloat64(487.62313),
			Status: types.OPEN,
		}
		balance.holdings["ESPORTS/USD"] = holding

		instrument := &Instrument{cache: map[string]kraken.InstrumentPair{}}
		pair := kraken.InstrumentPair{
			Symbol: "ESPORTS/USD",
			Base:   "ESPORTS",
			Quote:  "USD",
			Status: "online",
		}
		instrument.Remember(pair)

		price := NewPrice(nil)
		So(price.RememberFee("ESPORTS/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck(&kraken.Ticker{Data: []kraken.TickerData{{
			Symbol: "ESPORTS/USD",
			Bid:    decimal.NewFromFloat64(0.04533),
			Ask:    decimal.NewFromFloat64(0.04534),
			Last:   decimal.NewFromFloat64(0.04533),
		}}})

		desk := &Desk{
			balance:    balance,
			instrument: instrument,
			price:      price,
			positions:  map[string]*Position{},
			fillsBySymbol: map[string][]Fill{
				"ESPORTS/USD": {{
					ExecID: "PAPER-00004",
					Side:   "buy",
					Qty:    decimal.NewFromFloat64(487.62313),
					Price:  decimal.NewFromFloat64(0.0463),
					Fee:    decimal.NewFromFloat64(0.0587000723894),
				}},
			},
			historyReady: true,
		}

		Convey("AdoptOpen seeds entry economics and marks PnL", func() {
			desk.AdoptOpen()

			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryPrice.Float64(), ShouldAlmostEqual, 0.0463, 1e-9)
			So(holding.EntryFee, ShouldNotBeNil)
			So(holding.Mark, ShouldNotBeNil)
			So(holding.PnL, ShouldNotBeNil)
			So(holding.ReturnPct, ShouldNotBeNil)
			So(*holding.ReturnPct, ShouldBeLessThan, 0)
			So(*holding.ReturnPct, ShouldBeGreaterThan, -1)
		})
	})
}
