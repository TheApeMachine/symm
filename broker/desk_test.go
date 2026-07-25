package broker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type testDeskOptions struct {
	balance      *Balance
	price        *Price
	instrument   *Instrument
	positions    map[string]*Position
	fillsBySymbol map[string][]Fill
	historyReady bool
}

/*
testDesk constructs a Desk through NewDesk and applies only test-specific wiring
so production-managed fields stay on the constructor path.
*/
func testDesk(options testDeskOptions) *Desk {
	desk := NewDesk(
		context.Background(),
		nil,
		options.instrument,
		options.price,
		options.balance,
		config.Fixture().Trading,
	)

	if options.fillsBySymbol != nil {
		desk.fillsBySymbol = options.fillsBySymbol
	}

	desk.historyReady = options.historyReady

	if options.positions != nil {
		desk.positions = options.positions
	}

	return desk
}

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
		balance.StoreHolding(holding)

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

		desk := testDesk(testDeskOptions{
			balance: balance,
			price:   price,
			positions: map[string]*Position{
				"ESPORTS/USD": position,
			},
		})

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

				payload := <-ui
				So(json.Valid(payload), ShouldBeTrue)

				var frame struct {
					Holdings []struct {
						Symbol string  `json:"symbol"`
						Mark   float64 `json:"mark"`
					} `json:"holdings"`
				}

				So(json.Unmarshal(payload, &frame), ShouldBeNil)
				So(frame.Holdings, ShouldHaveLength, 1)
				So(frame.Holdings[0].Symbol, ShouldEqual, "ESPORTS/USD")
				So(frame.Holdings[0].Mark, ShouldAlmostEqual, 0.9, 1e-9)
			})
		})
	})
}

func TestDeskAdoptOpen(t *testing.T) {
	Convey("Given wallet holdings and a matching instrument pair", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.StoreHolding(&types.Holding{
			Symbol: "ETH/USD",
			Asset:  "ETH",
			Qty:    decimal.NewFromFloat64(3),
			Status: types.OPEN,
		})

		instrument := &Instrument{cache: map[string]kraken.InstrumentPair{}}
		instrument.Remember(kraken.InstrumentPair{
			Symbol: "ETH/USD",
			Base:   "ETH",
			Quote:  "USD",
			Status: "online",
		})

		desk := testDesk(testDeskOptions{
			balance:    balance,
			instrument: instrument,
		})

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
		balance.StoreHolding(holding)

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

		desk := testDesk(testDeskOptions{
			balance:    balance,
			instrument: instrument,
			price:      price,
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
		})

		Convey("AdoptOpen seeds entry economics and marks PnL", func() {
			desk.AdoptOpen()

			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryPrice.Float64(), ShouldAlmostEqual, 0.0463, 1e-9)
			So(holding.EntryFee, ShouldNotBeNil)
			So(holding.EntryFee.Float64(), ShouldAlmostEqual, 0.0587000723894, 1e-9)
			So(holding.Mark, ShouldNotBeNil)
			So(holding.PnL, ShouldNotBeNil)
			So(holding.ReturnPct, ShouldNotBeNil)
			So(*holding.ReturnPct, ShouldBeLessThan, 0)
			So(*holding.ReturnPct, ShouldBeGreaterThan, -1)
		})
	})
}
