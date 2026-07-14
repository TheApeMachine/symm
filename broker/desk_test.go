package broker

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestNewDesk(t *testing.T) {
	Convey("Given a newly constructed Desk", t, func() {
		desk := NewDesk(nil, nil, nil, make(chan []byte, 1))

		Convey("Then it waits for balance and price before hydrating", func() {
			So(desk.Status(), ShouldEqual, types.INITIALIZING)
		})
	})
}

func TestDeskPublish(t *testing.T) {
	Convey("Given a desk holding one open position", t, func() {
		ui := make(chan []byte, 1)
		desk := NewDesk(nil, nil, &Balance{}, ui)
		desk.positions.Store("BTC/USD", &Position{
			Data: &PositionData{
				Symbol:     "BTC/USD",
				Qty:        *decimal.NewFromFloat64(0.0001),
				EntryPrice: *decimal.NewFromFloat64(64129.900),
				Mark:       *decimal.NewFromFloat64(63039.400),
				PnL:        *decimal.NewFromFloat64(-0.142114),
				ReturnPct:  -0.0222,
			},
		})

		desk.Publish()

		Convey("It should publish flat position snapshots the frontend can parse", func() {
			var frame map[string]any

			select {
			case payload := <-ui:
				err := sonic.Unmarshal(payload, &frame)
				So(err, ShouldBeNil)
			default:
				t.Fatal("desk publish did not emit a frame")
			}

			positions, ok := frame["positions"].([]any)
			So(ok, ShouldBeTrue)
			So(positions, ShouldHaveLength, 1)

			position, ok := positions[0].(map[string]any)
			So(ok, ShouldBeTrue)
			So(position["symbol"], ShouldEqual, "BTC/USD")
		})
	})
}
