package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

type dummyTransport struct{}

func (d *dummyTransport) Read() ([]byte, error) {
	return nil, nil
}

func (d *dummyTransport) Write([]byte) error {
	return nil
}

func TestDesk(t *testing.T) {
	Convey("Given a Desk instance", t, func() {
		ctx := context.Background()
		desk := NewDesk(ctx, &dummyTransport{}, nil, nil)
		So(desk, ShouldNotBeNil)

		Convey("When Enter is called with a symbol and quantity", func() {
			qty := decimal.NewFromFloat64(0.5)
			err := desk.Enter("BTC/USD", qty)
			So(err, ShouldBeNil)

			val, ok := desk.positions.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			positions := val.([]*Position)
			So(len(positions), ShouldEqual, 1)

			Convey("When Apply is called with an execution", func() {
				desk.Apply(&kraken.Execution{
					Data: []kraken.ExecutionData{
						{
							Symbol:   "BTC/USD",
							ExecType: "trade",
							LastQty:  qty,
						},
					},
				})

				Convey("When Exit is called", func() {
					err := desk.Exit("BTC/USD")
					So(err, ShouldBeNil)
				})
			})
		})
	})
}
