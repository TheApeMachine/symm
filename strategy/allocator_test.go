package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestAllocatorAllocate proves exact exchange-quantized sizing for wallet,
risk-haircut, minimum-rejection, and rotation-capital regimes.
*/
func TestAllocatorAllocate(t *testing.T) {
	Convey("Given a funded allocator and one executable instrument", t, func() {
		previousFraction := viper.Get("trading.allocation.max_fraction")
		previousQuote := viper.Get("market.quote_currency")
		viper.Set("trading.allocation.max_fraction", 0.20)
		viper.Set("market.quote_currency", "USD")

		Reset(func() {
			viper.Set("trading.allocation.max_fraction", previousFraction)
			viper.Set("market.quote_currency", previousQuote)
		})

		for _, scenario := range []struct {
			name             string
			cash             string
			haircut          float64
			cause            string
			displacedCapital float64
			quantity         string
			notional         string
			reason           string
			holding          bool
		}{
			{
				name:     "normal entry consumes exactly one wallet slice",
				cash:     "5000",
				quantity: "19948",
				notional: "999.99324",
				reason:   "sized from wallet slice",
				holding:  true,
			},
			{
				name:     "haircut removes exactly half of the wallet slice",
				cash:     "5000",
				haircut:  0.50,
				quantity: "9974",
				notional: "499.99662",
				reason:   "sized from wallet slice",
				holding:  true,
			},
			{
				name:    "instrument minimum rejects an undersized wallet slice",
				cash:    "10",
				reason:  "minimum exceeds wallet slice",
				holding: false,
			},
			{
				name:             "rotation uses exact displaced capital without quote cash",
				cash:             "0",
				haircut:          0.25,
				cause:            "rotation",
				displacedCapital: 1000,
				quantity:         "14961",
				notional:         "749.99493",
				reason:           "sized from displaced capital",
				holding:          true,
			},
		} {
			scenario := scenario

			Convey("When "+scenario.name, func() {
				balance := broker.NewBalance(nil, nil, nil)
				balance.BalanceAck([]byte(
					`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
						`"asset":"USD","balance":"` + scenario.cash +
						`","available":"` + scenario.cash + `","reserved":"0"}]}`,
				))
				instrument := broker.NewInstrument(nil, nil, nil)
				instrument.Remember(&kraken.InstrumentPair{
					Symbol:        "LRC/USD",
					Base:          "LRC",
					Quote:         "USD",
					Status:        "online",
					QtyIncrement:  decimal.NewFromInt64(1),
					CostPrecision: 5,
					CostMin:       decimal.NewFromFloat64(0.5),
					QtyMin:        decimal.NewFromInt64(50),
				})
				price := broker.NewPrice(nil)
				err := price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
					Fee: decimal.NewFromFloat64(0.26),
				})
				So(err, ShouldBeNil)
				price.TickerAck([]byte(
					`{"channel":"ticker","type":"update","data":[{` +
						`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
				))

				allocator := NewAllocator(
					context.Background(),
					balance,
					instrument,
					price,
				)
				Reset(allocator.Close)
				thesis := types.NewThesis(nil)
				decision := types.Decision{
					Action:            types.ActionEnter,
					Symbol:            "LRC/USD",
					AllocationHaircut: scenario.haircut,
					Cause:             scenario.cause,
				}

				if scenario.displacedCapital > 0 {
					decision.ProposedNotional = decimal.NewFromFloat64(
						scenario.displacedCapital,
					)
				}

				thesis.Decisions = append(thesis.Decisions, decision)
				thesis.Holdings.Store("LRC/USD", &types.Holding{
					Symbol: "LRC/USD",
					Status: types.PENDING,
				})

				err = allocator.Allocate(thesis)

				So(err, ShouldBeNil)
				So(thesis.Decisions, ShouldHaveLength, 1)
				So(thesis.Decisions[0].Reason, ShouldEqual, scenario.reason)
				value, found := thesis.Holdings.Load("LRC/USD")
				So(found, ShouldEqual, scenario.holding)

				if !scenario.holding {
					So(thesis.Decisions[0].ProposedQuantity, ShouldBeNil)
					So(thesis.Decisions[0].ProposedNotional, ShouldBeNil)
					So(thesis.Decisions[0].ReferencePrice, ShouldBeNil)

					return
				}

				So(thesis.Decisions[0].ProposedQuantity.String(), ShouldEqual, scenario.quantity)
				So(thesis.Decisions[0].ProposedNotional.String(), ShouldEqual, scenario.notional)
				So(thesis.Decisions[0].ReferencePrice.String(), ShouldEqual, "0.05")
				So(value.(*types.Holding).Qty.String(), ShouldEqual, scenario.quantity)
				So(value.(*types.Holding).Status, ShouldEqual, types.PENDING)
			})
		}
	})

	Convey("Given no entry decision requires sizing", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"0","available":"0","reserved":"0"}]}`,
		))
		allocator := NewAllocator(
			t.Context(),
			balance,
			broker.NewInstrument(nil, nil, nil),
			broker.NewPrice(nil),
		)
		Reset(allocator.Close)
		thesis := types.NewThesis(nil)
		decision := types.Decision{
			Action: types.ActionExit,
			Symbol: "LRC/USD",
			Cause:  "stop",
			Reason: "mark returned through live stop",
		}
		thesis.Decisions = []types.Decision{decision}

		So(allocator.Allocate(thesis), ShouldBeNil)
		So(thesis.Decisions, ShouldResemble, []types.Decision{decision})
	})
}

/*
BenchmarkAllocatorAllocate measures the real allocation path with sizing state
reset before every iteration so no iteration becomes an already-sized no-op.
*/
func BenchmarkAllocatorAllocate(b *testing.B) {
	previousFraction := viper.Get("trading.allocation.max_fraction")
	previousQuote := viper.Get("market.quote_currency")
	viper.Set("trading.allocation.max_fraction", 0.20)
	viper.Set("market.quote_currency", "USD")
	b.Cleanup(func() {
		viper.Set("trading.allocation.max_fraction", previousFraction)
		viper.Set("market.quote_currency", previousQuote)
	})

	balance := broker.NewBalance(nil, nil, nil)
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.Remember(&kraken.InstrumentPair{
		Symbol:        "LRC/USD",
		Base:          "LRC",
		Quote:         "USD",
		Status:        "online",
		QtyIncrement:  decimal.NewFromInt64(1),
		CostPrecision: 5,
		CostMin:       decimal.NewFromFloat64(0.5),
		QtyMin:        decimal.NewFromInt64(50),
	})
	price := broker.NewPrice(nil)
	err := price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})

	if err != nil {
		b.Fatal(err)
	}

	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
	))
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	b.Cleanup(allocator.Close)
	thesis := types.NewThesis(nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: types.ActionEnter,
		Symbol: "LRC/USD",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
		))
		thesis.Decisions[0].ProposedQuantity = nil
		thesis.Decisions[0].ProposedNotional = nil
		thesis.Decisions[0].ReferencePrice = nil
		thesis.Holdings.Store("LRC/USD", &types.Holding{
			Symbol: "LRC/USD",
			Status: types.PENDING,
		})

		if err := allocator.Allocate(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
