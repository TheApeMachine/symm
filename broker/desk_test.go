package broker

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskInitialize verifies restart management comes from the persisted Thesis
Holding while its quantity is refreshed from the authoritative wallet snapshot.
*/
func TestDeskInitialize(t *testing.T) {
	Convey("Given a persisted open Holding and its current wallet balance", t, func() {
		ctx := context.Background()
		mock := tests.NewMockAPI()
		paper := websocket.NewPaper(
			ctx, websocket.NewLatencySimulator(system.NewBooter(ctx, nil)),
		)
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), paper)
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
			Symbol: "BTC/USD", Base: "BTC", Quote: "USD",
		})
		balance := &Balance{quote: "USD", holdings: &sync.Map{}}
		balance.Update("BTC", types.Holding{
			Asset: "BTC", Qty: decimal.NewFromFloat64(0.4),
		})
		thesis := types.NewThesis(nil)
		thesis.Positions = append(thesis.Positions, types.Holding{
			Symbol: "BTC/USD", Asset: "BTC", Qty: decimal.NewFromFloat64(0.25),
			Order: &spot.Order{
				Description: &spot.OrderDescription{Pair: "BTC/USD"},
				Volume:      decimal.NewFromFloat64(0.25),
			},
		})
		desk := NewDesk(api, instrument, &Price{}, balance, thesis, nil)

		err := desk.Initialize()

		Convey("It should restore one manager at the wallet quantity", func() {
			So(err, ShouldBeNil)
			So(desk.Status(), ShouldEqual, types.READY)
			So(desk.OpenPositions(), ShouldEqual, 1)
			holding, holdingErr := balance.Holding("BTC/USD")
			So(holdingErr, ShouldBeNil)
			So(holding.Qty.Float64(), ShouldEqual, 0.4)
			So(holding.Order.Volume.Float64(), ShouldEqual, 0.4)
		})
	})
}
