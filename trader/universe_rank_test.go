package trader

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"

	. "github.com/smartystreets/goconvey/convey"
)

/*
universeFeeConn is a minimal websocket.Conn that serves a canned
TradeVolume fee schedule from Post, letting tests populate a real
broker.Price through its actual broker.NewFees/broker.NewQuote callbacks
instead of poking unexported fields from another package.
*/
type universeFeeConn struct {
	feeSchedule []byte
}

func (conn *universeFeeConn) Client() *spot.WebSocket    { return nil }
func (conn *universeFeeConn) On(string, func([]byte))    {}
func (conn *universeFeeConn) Write(json.Marshaler) error { return nil }
func (conn *universeFeeConn) Close()                     {}

func (conn *universeFeeConn) Get(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (conn *universeFeeConn) Post(string, json.Marshaler) ([]byte, error) {
	return conn.feeSchedule, nil
}

var _ websocket.Conn = (*universeFeeConn)(nil)

/*
newUniversePrice builds a real broker.Price by driving its actual
instrument/ticker callbacks, so ranking tests exercise the same fee and
quote plumbing production code uses.
*/
func newUniversePrice(
	t testing.TB,
	fees map[string]kraken.FeeRates,
	tickerJSON string,
) *broker.Price {
	schedule, err := sonic.Marshal(kraken.FeeSchedule{Pairs: fees})
	if err != nil {
		t.Fatal(err)
	}

	conn := &universeFeeConn{feeSchedule: schedule}
	price := broker.NewPrice(conn, conn)

	pairs := make([]map[string]any, 0, len(fees))

	for symbol := range fees {
		pairs = append(pairs, map[string]any{
			"symbol": symbol,
			"quote":  "USD",
			"status": "online",
		})
	}

	instrumentBody, err := sonic.Marshal(map[string]any{"pairs": pairs})
	if err != nil {
		t.Fatal(err)
	}

	broker.NewFees(price).On(instrumentBody)
	broker.NewQuote(price).On([]byte(tickerJSON))

	return price
}

func TestUniverseRankerRank(t *testing.T) {
	Convey("Given four symbols with distinct liquidity, depth, and cost", t, func() {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		tickerJSON := `[
			{"symbol":"AAA/USD","bid":100,"ask":100.01,"bid_qty":1000,"ask_qty":1000,"last":100,"volume":1000000,"vwap":100,"timestamp":"` + now + `"},
			{"symbol":"CCC/USD","bid":50,"ask":50.1,"bid_qty":100,"ask_qty":100,"last":50,"volume":100000,"vwap":50,"timestamp":"` + now + `"},
			{"symbol":"BBB/USD","bid":10,"ask":10.5,"bid_qty":5,"ask_qty":5,"last":10,"volume":1000,"vwap":10,"timestamp":"` + now + `"},
			{"symbol":"DDD/USD","bid":1,"ask":2,"bid_qty":1,"ask_qty":1,"last":1,"volume":1,"vwap":1,"timestamp":"` + now + `"}
		]`
		price := newUniversePrice(t, map[string]kraken.FeeRates{
			"AAA/USD": {Taker: 0.0001},
			"CCC/USD": {Taker: 0.002},
			"BBB/USD": {Taker: 0.01},
			"DDD/USD": {Taker: 0.01},
		}, tickerJSON)
		ranker := newUniverseRanker(price, time.Hour)

		Convey("When ranking the observed snapshot", func() {
			snapshot := map[string]kraken.TickerData{}

			for _, row := range kraken.NewTickerDataSlice([]byte(tickerJSON)) {
				snapshot[row.Symbol] = row
			}

			ordered := ranker.rank(snapshot)

			Convey("Then symbols are ordered best liquidity/depth/cost first", func() {
				So(ordered, ShouldResemble, []string{"AAA/USD", "CCC/USD", "BBB/USD", "DDD/USD"})
			})
		})

		Convey("When a symbol's quote is stale", func() {
			snapshot := map[string]kraken.TickerData{}

			for _, row := range kraken.NewTickerDataSlice([]byte(tickerJSON)) {
				if row.Symbol == "AAA/USD" {
					row.Timestamp = time.Now().Add(-2 * time.Hour)
				}

				snapshot[row.Symbol] = row
			}

			ordered := ranker.rank(snapshot)

			Convey("Then it is excluded from ranking", func() {
				So(ordered, ShouldNotContain, "AAA/USD")
				So(ordered, ShouldContain, "CCC/USD")
			})
		})

		Convey("When a symbol has no fee schedule entry", func() {
			priceWithoutFee := newUniversePrice(t, map[string]kraken.FeeRates{
				"AAA/USD": {Taker: 0.0001},
			}, `[{"symbol":"AAA/USD","bid":100,"ask":100.01,"bid_qty":1000,"ask_qty":1000,"last":100,"volume":1000000,"vwap":100,"timestamp":"`+now+`"},
				{"symbol":"CCC/USD","bid":50,"ask":50.1,"bid_qty":100,"ask_qty":100,"last":50,"volume":100000,"vwap":50,"timestamp":"`+now+`"}]`)
			unfeeRanker := newUniverseRanker(priceWithoutFee, time.Hour)

			ordered := unfeeRanker.rank(map[string]kraken.TickerData{
				"AAA/USD": {
					Symbol: "AAA/USD", Volume: 1000000, Vwap: 100,
					Bid: *decimal.NewFromFloat64(100), Ask: *decimal.NewFromFloat64(100.01),
					BidQty: 1000, AskQty: 1000, Timestamp: time.Now(),
				},
				"CCC/USD": {
					Symbol: "CCC/USD", Volume: 100000, Vwap: 50,
					Bid: *decimal.NewFromFloat64(50), Ask: *decimal.NewFromFloat64(50.1),
					BidQty: 100, AskQty: 100, Timestamp: time.Now(),
				},
			})

			Convey("Then it is excluded since no round-trip friction can be derived", func() {
				So(ordered, ShouldResemble, []string{"AAA/USD"})
			})
		})
	})
}

/*
BenchmarkUniverseRankerRank measures ranking cost across an observation
tier sized like a real Kraken USD pair catalog, so subscription rebalance
cadence can be chosen with a known compute budget per rebalance.
*/
func BenchmarkUniverseRankerRank(b *testing.B) {
	const symbolCount = 500

	now := time.Now().UTC().Format(time.RFC3339Nano)
	fees := make(map[string]kraken.FeeRates, symbolCount)
	rows := make([]map[string]any, 0, symbolCount)

	for index := range symbolCount {
		symbol := fmt.Sprintf("SYM%03d/USD", index)
		fees[symbol] = kraken.FeeRates{Taker: 0.0001 * float64(index%50+1)}
		rows = append(rows, map[string]any{
			"symbol":    symbol,
			"bid":       100.0 - float64(index%100)*0.1,
			"ask":       100.1 - float64(index%100)*0.1,
			"bid_qty":   1000.0,
			"ask_qty":   1000.0,
			"last":      100.0,
			"volume":    float64(1000 + index),
			"vwap":      100.0,
			"timestamp": now,
		})
	}

	tickerBody, err := sonic.Marshal(rows)
	if err != nil {
		b.Fatal(err)
	}

	price := newUniversePrice(b, fees, string(tickerBody))
	ranker := newUniverseRanker(price, time.Hour)

	snapshot := map[string]kraken.TickerData{}
	for _, row := range kraken.NewTickerDataSlice(tickerBody) {
		snapshot[row.Symbol] = row
	}

	b.ReportAllocs()

	for b.Loop() {
		ranker.rank(snapshot)
	}
}
