package broker

import (
	"context"
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
			closed := position.onExecution(kraken.Execution{Data: []kraken.ExecutionData{{
				ClientOrderID: "exit",
				OrderStatus:   "filled",
			}}})

			So(closed, ShouldBeTrue)
			stored, err = store.Load(t.Context(), "SIM/USD")
			So(err, ShouldBeNil)
			So(stored, ShouldBeNil)
		})
	})
}
