package broker

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
fillingTransport fills every market order at a fixed unit price with a fixed
fee, echoing the client order ID back through Desk.Apply like Paper does.
*/
type fillingTransport struct {
	desk  *Desk
	unit  map[string]float64
	fee   float64
	write chan struct{}
}

func (transport *fillingTransport) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}

	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}

	quantity, err := decimal.NewFromString(message.Params.Volume)

	if err != nil {
		return err
	}

	unit := transport.unit[message.Params.Type]

	transport.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: message.Params.ClOrdId,
		Symbol:        message.Params.Pair,
		Side:          message.Params.Type,
		LastQty:       quantity,
		Cost:          decimal.NewFromFloat64(quantity.Float64() * unit),
		FeeUsdEquiv:   decimal.NewFromFloat64(transport.fee),
	}}})

	transport.write <- struct{}{}
	return nil
}

func TestDesk_Apply(t *testing.T) {
	Convey("Given a Desk trading through a transport that fills market orders", t, func() {
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
			},
		})

		price := NewPrice(context.Background(), nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		transport := &fillingTransport{
			unit:  map[string]float64{"buy": 60000, "sell": 63000},
			fee:   0.1,
			write: make(chan struct{}, 1),
		}

		desk := NewDesk(context.Background(), transport, price)
		transport.desk = desk

		var closures []Closure
		desk.OnClose(func(closure Closure) { closures = append(closures, closure) })

		Convey("A full round trip closes with realized PnL from the venue fills", func() {
			So(desk.Enter("BTC/USD"), ShouldBeNil)
			<-transport.write
			So(desk.State("BTC/USD"), ShouldEqual, HOLDING)

			So(desk.Exit("BTC/USD"), ShouldBeNil)
			<-transport.write
			So(desk.State("BTC/USD"), ShouldEqual, FLAT)

			So(closures, ShouldHaveLength, 1)
			closure := closures[0]
			expected := closure.Proceeds.Sub(closure.Cost).Sub(closure.Fees)
			So(closure.Realized.Cmp(expected), ShouldEqual, 0)
			So(closure.Realized.Sign(), ShouldBeGreaterThan, 0)
		})

		Convey("A second entry on an open symbol is refused", func() {
			So(desk.Enter("BTC/USD"), ShouldBeNil)
			<-transport.write
			So(desk.Enter("BTC/USD"), ShouldNotBeNil)
		})
	})
}
