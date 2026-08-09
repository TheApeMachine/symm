package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPositionOnTicker(t *testing.T) {
	Convey("Given a position whose entry has not filled", t, func() {
		price, _ := newPriceSurface(t, "SIM/USD")
		forecast, err := types.NewResonanceForecast(
			[]float64{-0.01},
			[]float64{1},
			1,
			0.9,
		)
		So(err, ShouldBeNil)

		zeroRate := decimal.NewFromInt64(0)
		stoploss, err := types.NewStoploss(
			context.Background(),
			"SIM/USD",
			decimal.NewFromFloat64(100.02),
			decimal.NewFromFloat64(100),
			forecast,
			decimal.NewFromFloat64(0.01),
			zeroRate,
			zeroRate,
		)
		So(err, ShouldBeNil)

		position := &Position{
			Status: types.PENDING,
			price:  price,
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(0),
				Mark:     decimal.NewFromFloat64(100),
				Stoploss: stoploss,
			},
		}

		Convey("It should remember the mark without triggering an unowned lot", func() {
			position.onTicker(kraken.TickerData{
				Bid: decimal.NewFromFloat64(98),
			})

			So(position.Holding.Mark.Cmp(decimal.NewFromFloat64(98)), ShouldEqual, 0)
			So(stoploss.Mark.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(stoploss.Status, ShouldEqual, types.ARMED)
		})
	})
}

func TestPositionOnExecution(t *testing.T) {
	Convey("Given a position with a strategy stoploss", t, func() {
		store := newPositionStoreFixture(t)
		price, _ := newPriceSurface(t, "SIM/USD")
		price.Update(&kraken.TickerData{
			Symbol: "SIM/USD",
			Bid:    decimal.NewFromFloat64(100),
			Ask:    decimal.NewFromFloat64(100.02),
		})
		position := &Position{
			price: price,
			store: store,
			ui:    make(chan []byte, 1),
			pair: kraken.InstrumentPair{
				Symbol: "SIM/USD",
				Base:   "SIM",
				Quote:  "USD",
			},
			EntryOrder: &spot.AddOrderRequest{ClOrdId: "entry"},
			Holding: &types.Holding{
				Mark:     decimal.NewFromFloat64(100),
				Stoploss: newBrokerStoploss(t),
			},
		}

		Convey("It should save on entry and delete on exit", func() {
			position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: "entry",
				OrderStatus:   "filled",
				Timestamp:     time.Now().UTC(),
				CumQty:        decimal.NewFromFloat64(2),
				CumCost:       decimal.NewFromFloat64(200.04),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.20),
			}}})

			stored, err := store.Load(t.Context(), "SIM/USD")
			So(err, ShouldBeNil)
			So(stored, ShouldNotBeNil)

			position.ExitOrder = &spot.AddOrderRequest{ClOrdId: "exit"}
			exitAt := time.Now().UTC()
			closed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: "exit",
				OrderStatus:   "filled",
				Timestamp:     exitAt,
				CumQty:        decimal.NewFromFloat64(2),
				CumCost:       decimal.NewFromFloat64(220),
				FeeUsdEquiv:   decimal.NewFromFloat64(0.22),
			}}})

			So(closed, ShouldBeTrue)
			So(position.Status, ShouldEqual, types.CLOSED)
			So(position.Holding.Status, ShouldEqual, types.CLOSED)
			So(position.Holding.ExitAt.Equal(exitAt), ShouldBeTrue)
			So(position.Holding.ExitPrice.Cmp(
				decimal.NewFromFloat64(110),
			), ShouldEqual, 0)
			So(position.Holding.PnL.Cmp(
				decimal.NewFromFloat64(19.54),
			), ShouldEqual, 0)
			frame := struct {
				Positions []struct {
					Status types.Status `json:"status"`
				} `json:"positions"`
			}{}
			So(json.Unmarshal(<-position.ui, &frame), ShouldBeNil)
			So(frame.Positions, ShouldHaveLength, 1)
			So(frame.Positions[0].Status, ShouldEqual, types.CLOSED)
			stored, err = store.Load(t.Context(), "SIM/USD")
			So(err, ShouldBeNil)
			So(stored, ShouldBeNil)
		})
	})
}

func TestPositionExit(t *testing.T) {
	Convey("Given an open lot whose regulator remains armed", t, func() {
		position := &Position{
			Status: types.OPEN,
			Holding: &types.Holding{
				Qty:      decimal.NewFromInt64(1),
				Stoploss: newBrokerStoploss(t),
			},
		}

		Convey("It should reject liquidation at the order boundary", func() {
			returned, err := position.Exit()

			So(returned, ShouldEqual, position)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "triggered stoploss required")
			So(position.ExitOrder, ShouldBeNil)
		})
	})
}

func TestPositionCloseFill(t *testing.T) {
	Convey("Given an exact filled exit for a tiny high-priced lot", t, func() {
		quantity := decimal.NewFromFloat64(0.00051057)
		entryPrice := decimal.NewFromFloat64(64951.1)
		entryGross := decimal.ExactMul(entryPrice, quantity)
		entryFee := decimal.ExactMul(
			entryGross,
			decimal.NewFromFloat64(0.0025),
		)
		exitPrice := decimal.NewFromFloat64(66000)
		exitGross := decimal.ExactMul(exitPrice, quantity)
		exitFee := decimal.ExactMul(
			exitGross,
			decimal.NewFromFloat64(0.0025),
		)
		position := &Position{
			Status: types.OPEN,
			ui:     make(chan []byte, 1),
			Holding: &types.Holding{
				Status:      types.OPEN,
				Qty:         quantity,
				SellableQty: quantity,
				EntryPrice:  entryPrice,
				EntryFee:    entryFee,
			},
		}
		execution := kraken.ExecutionData{
			Timestamp:   time.Now().UTC(),
			CumQty:      quantity,
			CumCost:     exitGross,
			FeeUsdEquiv: exitFee,
		}

		Convey("It should retain realized proceeds and fees before publishing closed", func() {
			err := position.closeFill(execution)
			expectedPnL := 66000*quantity.Float64()*(1-0.0025) -
				64951.1*quantity.Float64()*(1+0.0025)

			So(err, ShouldBeNil)
			So(position.Holding.ExitPrice.Cmp(exitPrice), ShouldEqual, 0)
			So(position.Holding.PnL.Float64(), ShouldAlmostEqual, expectedPnL, 1e-10)
			So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0)
			So(position.Holding.ReturnPct, ShouldBeLessThan, 2)
			So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
			So(position.Status, ShouldEqual, types.CLOSED)
		})

		Convey("It should refuse a filled quantity that does not match inventory", func() {
			execution.CumQty = quantity.Add(quantity.GetSmallestIncrement())

			err := position.closeFill(execution)

			So(err, ShouldNotBeNil)
			So(position.Status, ShouldEqual, types.OPEN)
			So(position.Holding.ExitPrice, ShouldBeNil)
		})

		Convey("It should refuse a filled event without realized cost", func() {
			execution.CumCost = nil

			err := position.closeFill(execution)

			So(err, ShouldNotBeNil)
			So(position.Status, ShouldEqual, types.OPEN)
			So(position.Holding.ExitPrice, ShouldBeNil)
		})
	})
}
