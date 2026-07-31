package toxicity

import (
	"encoding/json"
	"testing"

	sdkbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	marketkraken "github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

type connAdapter struct {
	*mockapi.MockConn
}

func (conn connAdapter) Status() types.Status {
	return types.READY
}

func (conn connAdapter) Subscribe(
	_ string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return subscription
}

func (conn connAdapter) Books() map[string]*sdkbook.Book {
	books := map[string]*sdkbook.Book{}

	for _, symbol := range conn.MockConn.Books().GetBooks() {
		books[symbol] = conn.MockConn.Books().GetBook(symbol)
	}

	return books
}

func (conn connAdapter) Book(symbol string) *sdkbook.Book {
	return conn.MockConn.Books().GetBook(symbol)
}

func (conn connAdapter) SubInstrument(types.Subscription[any]) {}
func (conn connAdapter) SubTicker([]string)                    {}
func (conn connAdapter) SubBook([]string)                      {}
func (conn connAdapter) SubTrades([]string)                    {}
func (conn connAdapter) SubL3([]string)                        {}
func (conn connAdapter) SubCandles([]string)                   {}

func (conn connAdapter) Balance() (map[string]*decimal.Decimal, error) {
	return nil, nil
}

func (conn connAdapter) TradeBalance() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}

func (conn connAdapter) TradeVolume([]string) (*marketkraken.TradeVolumeResult, error) {
	return nil, nil
}

func (conn connAdapter) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return spot.AddOrderResult{}, nil
}

func (conn connAdapter) Write(params json.Marshaler, _ ...websocket.Callback[any]) error {
	return conn.MockConn.Write(params)
}

func drainToxicityTrades(sub *types.Subscription[any]) []kraken.TradeData {
	out := make([]kraken.TradeData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if trade, ok := frame.(*kraken.Trade); ok {
				out = append(out, trade.Data...)
			}
		default:
			return out
		}
	}
}

func drainToxicityBooks(sub *types.Subscription[any]) []kraken.BookData {
	out := make([]kraken.BookData, 0)

	for {
		select {
		case frame := <-sub.Channel:
			if book, ok := frame.(*kraken.Book); ok {
				out = append(out, book.Data...)
			}
		default:
			return out
		}
	}
}

func measureToxicity(t *testing.T, state tests.MarketState, focus ...string) []*types.Measurement {
	market := tests.NewMarket(t.Context(), 3)
	So(market.Bootstrap(), ShouldBeNil)
	defer market.Close()

	api := websocket.NewAPI(t.Context(), connAdapter{market.Public}, connAdapter{market.Level3})
	thesis := types.NewThesis()
	thesis.Causal.Store("signal:toxicity:touch", make(map[string]float64))
	thesis.Causal.Store("signal:toxicity:price", make(map[string]float64))
	signal := &Signal{thesis: thesis, api: api, ui: make(chan []byte, 32)}
	tradeSub := market.Public.Subscribe("trade")
	bookSub := market.Public.Subscribe("book")

	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"trade","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Public.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"book","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"]}}`,
	)), ShouldBeNil)
	So(market.Level3.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"level3","symbol":["SIM1/USD","SIM2/USD","SIM3/USD"],"depth":10}}`,
	)), ShouldBeNil)

	signal.thesis.Tick++
	signal.Calculate(drainToxicityBooks(bookSub), drainToxicityTrades(tradeSub))

	consume := func(into *[]*types.Measurement) func() error {
		return func() error {
			signal.thesis.Tick++
			*into = append(*into, signal.Calculate(
				drainToxicityBooks(bookSub),
				drainToxicityTrades(tradeSub),
			)...)

			return nil
		}
	}

	So(market.Warmup(consume(&[]*types.Measurement{})), ShouldBeNil)
	rows := make([]*types.Measurement, 0)
	So(market.Transition(state, consume(&rows), focus...), ShouldBeNil)

	return rows
}

func TestCalculate(t *testing.T) {
	Convey("Toxicity preserves touch and trade-side evidence on live market fixtures", t, func() {
		rows := measureToxicity(t, tests.MarketStateFastPump)
		So(rows, ShouldNotBeEmpty)

		foundTrade := false
		foundTouch := false

		for _, measurement := range rows {
			measurement.EachMetric(func(metric types.MetricType, side types.MeasurementSide, sample types.MetricSample) bool {
				if metric == types.MetricTradeVolume && side == types.SideNone && sample.Raw > 0 {
					foundTrade = true
				}

				if metric == types.MetricTouchQuantity && (side == types.SideBuy || side == types.SideSell) && sample.Raw > 0 {
					foundTouch = true
				}

				return true
			})
		}

		So(foundTrade, ShouldBeTrue)
		So(foundTouch, ShouldBeTrue)
	})
}
