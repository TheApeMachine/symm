package broker

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	book "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type deskConn struct {
	orders int
}

func (conn *deskConn) Status() types.Status { return types.READY }
func (conn *deskConn) Subscribe(
	_ string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return subscription
}
func (conn *deskConn) Books() *sync.Map                      { return &sync.Map{} }
func (conn *deskConn) Book(string) *book.Book                { return nil }
func (conn *deskConn) SubInstrument(types.Subscription[any]) {}
func (conn *deskConn) SubTicker([]string)                    {}
func (conn *deskConn) SubBook([]string)                      {}
func (conn *deskConn) SubTrades([]string)                    {}
func (conn *deskConn) SubL3([]string)                        {}
func (conn *deskConn) SubCandles([]string)                   {}
func (conn *deskConn) Balance() (map[string]*decimal.Decimal, error) {
	return map[string]*decimal.Decimal{"USD": decimal.NewFromInt64(100)}, nil
}
func (conn *deskConn) TradeBalance() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}
func (conn *deskConn) TradeVolume([]string) (*kraken.TradeVolumeResult, error) {
	return &kraken.TradeVolumeResult{}, nil
}
func (conn *deskConn) AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error) {
	conn.orders++
	return spot.AddOrderResult{OrderPlacementSingle: spot.OrderPlacementSingle{
		ID: []string{"venue-order"},
	}}, nil
}
func (conn *deskConn) Write(json.Marshaler, ...websocket.Callback[any]) error { return nil }
func (conn *deskConn) Post(string, json.Marshaler) ([]byte, error)            { return nil, nil }
func (conn *deskConn) Client() *spot.WebSocket                                { return nil }
func (conn *deskConn) Close()                                                 {}

func TestDeskExecute(t *testing.T) {
	Convey("Given an executable entry decision", t, func() {
		ctx := t.Context()
		conn := &deskConn{}
		api := websocket.NewAPI(ctx, conn, conn)
		pair := kraken.InstrumentPair{Symbol: "BTC/USD"}
		api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
				"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
			},
			NewPairs: map[string]spot.AssetPair{
				pair.Symbol: {
					WSName:        pair.Symbol,
					Base:          "BTC",
					Quote:         "USD",
					PairDecimals:  2,
					LotDecimals:   8,
					LotMultiplier: 1,
					TickSize:      decimal.NewFromFloat64(0.01),
				},
			},
		})
		instrument := &Instrument{cache: &sync.Map{}}
		instrument.cache.Store(pair.Symbol, pair)
		price := &Price{
			api:        api,
			tickers:    &sync.Map{},
			fees:       &sync.Map{},
			normalizer: api.Normalizer(),
		}
		price.tickers.Store(pair.Symbol, &kraken.TickerData{
			Symbol: pair.Symbol,
			Bid:    decimal.NewFromInt64(100),
			Ask:    decimal.NewFromInt64(101),
		})
		price.fees.Store(pair.Symbol, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.0026),
		})
		balance := &Balance{wallet: &sync.Map{}, quote: "USD"}
		balance.wallet.Store("USD", decimal.NewFromInt64(100))
		desk := &Desk{
			ctx:        context.Background(),
			api:        api,
			ui:         make(chan []byte, 8),
			instrument: instrument,
			price:      price,
			balance:    balance,
			positions:  &sync.Map{},
		}
		decision := types.Decision{
			ID:               uuid.NewString(),
			Action:           types.ActionEnter,
			Symbol:           pair.Symbol,
			ProposedQuantity: decimal.NewFromFloat64(0.25),
			ProposedNotional: decimal.NewFromInt64(25),
		}

		Convey("Execute should submit and retain the position before a fill", func() {
			So(desk.Execute([]types.Decision{decision}), ShouldBeNil)
			So(conn.orders, ShouldEqual, 1)
			So(desk.OpenPositions(), ShouldEqual, 1)

			positions := slices.Collect(desk.Positions())
			So(positions, ShouldHaveLength, 1)
			So(positions[0].ID, ShouldEqual, decision.ID)
			So(positions[0].EntryOrder.ClOrdId, ShouldEqual, decision.ID)
			So(positions[0].EntryOrderID, ShouldEqual, "venue-order")
			So(positions[0].Status, ShouldEqual, types.PENDING)
		})
	})
}
