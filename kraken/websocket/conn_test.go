package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

type tradeVolumeConn struct {
	client           *spot.WebSocket
	tradeVolumeInput []string
	tradeVolume      *kraken.TradeVolumeResult
}

func (tradeVolumeConn *tradeVolumeConn) Status() types.Status { return types.READY }

func (tradeVolumeConn *tradeVolumeConn) Subscribe(string, *types.Subscription[any]) *types.Subscription[any] {
	return nil
}

func (tradeVolumeConn *tradeVolumeConn) Books() *sync.Map { return nil }

func (tradeVolumeConn *tradeVolumeConn) Book(string) *book.Book { return nil }

func (tradeVolumeConn *tradeVolumeConn) SubInstrument(types.Subscription[any]) {}

func (tradeVolumeConn *tradeVolumeConn) SubTicker([]string) {}

func (tradeVolumeConn *tradeVolumeConn) SubBook([]string) {}

func (tradeVolumeConn *tradeVolumeConn) SubTrades([]string) {}

func (tradeVolumeConn *tradeVolumeConn) SubL3([]string) {}

func (tradeVolumeConn *tradeVolumeConn) SubCandles([]string) {}

func (tradeVolumeConn *tradeVolumeConn) Balance() (map[string]*decimal.Decimal, error) {
	return nil, nil
}

func (tradeVolumeConn *tradeVolumeConn) TradeBalance() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}

func (tradeVolumeConn *tradeVolumeConn) TradeVolume(symbols []string) (*kraken.TradeVolumeResult, error) {
	tradeVolumeConn.tradeVolumeInput = append([]string{}, symbols...)
	return tradeVolumeConn.tradeVolume, nil
}

func (tradeVolumeConn *tradeVolumeConn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return spot.AddOrderResult{}, nil
}

func (tradeVolumeConn *tradeVolumeConn) Write(json.Marshaler, ...Callback[any]) error { return nil }

func (tradeVolumeConn *tradeVolumeConn) Post(string, json.Marshaler) ([]byte, error) { return nil, nil }

func (tradeVolumeConn *tradeVolumeConn) Client() *spot.WebSocket { return tradeVolumeConn.client }

func (tradeVolumeConn *tradeVolumeConn) Close() {}

func mustConnDecimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)

	if err != nil {
		panic(err)
	}

	return parsed
}

func TestTradeVolume(t *testing.T) {
	Convey("Given a Kraken API with a normalizer and private trade volume response keyed by normalized pair name", t, func() {
		mockConn := mockapi.NewConn(context.Background(), "BTC/USD")
		private := &tradeVolumeConn{
			client: mockConn.Client(),
			tradeVolume: &kraken.TradeVolumeResult{
				Fees: map[string]kraken.TradeVolumeFee{
					"XXBT/ZUSD": {
						Fee: mustConnDecimal("0.2600"),
					},
				},
			},
		}
		api := NewAPI(context.Background(), private, private)

		Convey("TradeVolume should use the normalizer canonical name for request and fee lookup", func() {
			result, err := api.TradeVolume([]string{"BTC/USD"})

			So(err, ShouldBeNil)
			So(private.tradeVolumeInput, ShouldResemble, []string{"BTC/USD"})
			So(result, ShouldNotBeNil)

			fee, ok := result.Fees["XXBT/ZUSD"]
			So(ok, ShouldBeTrue)
			So(fee.Fee.String(), ShouldEqual, "0.2600")
		})
	})
}
