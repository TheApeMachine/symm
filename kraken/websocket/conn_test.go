package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

func TestAPITradeVolume(t *testing.T) {
	Convey("Given a private TradeVolume response keyed by the requested pair", t, func() {
		viper.Set("trading.model", "live")
		public := &stubConn{client: normalizerClient()}
		private := &stubConn{postResponse: []byte(`{
			"error":[],
			"result":{
				"fees":{"XXBTZUSD":{"fee":"0.2600","min_fee":"0.1000","tier_volume":"0.0000"}},
				"fees_maker":{"XXBTZUSD":{"fee":"0.1600"}}
			}
		}`)}
		api := NewAPI(context.Background(), public, private, nil)

		Convey("When the fee tier is requested", func() {
			tradeVolume, err := api.TradeVolume([]string{"BTC/USD"})

			Convey("Then the private endpoint is used and the response passes through unmodified", func() {
				So(err, ShouldBeNil)
				So(private.postPath, ShouldEqual, TradeVolumeEndpoint)
				So(tradeVolume.Result.Fees, ShouldResemble, map[string]kraken.TradeVolumeFees{
					"XXBTZUSD": {
						Fee: "0.2600", MinFee: "0.1000", TierVolume: "0.0000",
					},
				})
				So(tradeVolume.Result.FeesMaker, ShouldResemble, map[string]kraken.TradeVolumeFees{
					"XXBTZUSD": {Fee: "0.1600"},
				})
			})
		})
	})
}

func BenchmarkAPITradeVolume(b *testing.B) {
	fees := make(map[string]kraken.TradeVolumeFees, 40)
	symbols := make([]string, 40)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ASSET-%02d/USD", index)
		fees[symbols[index]] = kraken.TradeVolumeFees{Fee: "0.2600"}
	}

	response, err := json.Marshal(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: fees},
	})

	if err != nil {
		b.Fatal(err)
	}

	private := &stubConn{postResponse: response}
	api := NewAPI(context.Background(), &stubConn{}, private, nil)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tradeVolume, err := api.TradeVolume(symbols)

		if err != nil {
			b.Fatal(err)
		}

		if len(tradeVolume.Result.Fees) != len(symbols) {
			b.Fatal("incomplete fee tier")
		}
	}
}

func TestAPIOnRoutesChannels(t *testing.T) {
	Convey("Given a live API with stub transports", t, func() {
		viper.Set("trading.model", "live")

		public := &stubConn{}
		private := &stubConn{}
		api := NewAPI(context.Background(), public, private, nil)

		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})
		api.On("level3", func([]byte) {})

		Convey("Then each callback registers on its dedicated transport", func() {
			So(len(public.channels["ticker"]), ShouldEqual, 1)
			So(len(private.channels["balances"]), ShouldEqual, 1)
			So(api.level3, ShouldHaveLength, 1)
			So(len(public.channels["level3"]), ShouldEqual, 0)
		})

		Convey("Then a level3 transport created before another consumer receives it", func() {
			level3 := New(context.Background(), nil, true, Level3WebSocketURL)
			api.bookConns.Store("BTC/USD", level3)
			api.On("level3", func([]byte) {})

			callbacks, ok := level3.sync.Load("level3")
			So(ok, ShouldBeTrue)
			So(callbacks.([]func([]byte)), ShouldHaveLength, 1)
		})
	})

	Convey("Given a paper API with stub transports", t, func() {
		viper.Set("trading.model", "paper")

		public := &stubConn{}
		private := &stubConn{}
		paper := NewPaper(context.Background(), newTestSimulator())
		api := NewAPI(context.Background(), public, private, paper)

		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})
		api.On("executions", func([]byte) {})
		api.On("add_order", func([]byte) {})

		Convey("Then ticker registers on the public transport", func() {
			So(len(public.channels["ticker"]), ShouldEqual, 1)
		})

		Convey("Then balances, executions, and add_order register on the paper transport instead", func() {
			So(len(private.channels["balances"]), ShouldEqual, 0)

			_, ok := paper.sync.Load("balances")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("executions")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("add_order")
			So(ok, ShouldBeTrue)
		})
	})
}

func TestAPIClose(t *testing.T) {
	Convey("Given an API with stub public and private transports", t, func() {
		public := &stubConn{}
		private := &stubConn{}
		api := NewAPI(context.Background(), public, private, nil)

		Convey("When the API closes", func() {
			api.Close()

			Convey("Then each transport closes exactly once", func() {
				So(public.closeCount, ShouldEqual, 1)
				So(private.closeCount, ShouldEqual, 1)
			})
		})
	})
}

func newTestSimulator() *Simulator {
	simulator := NewSimulator()
	simulator.Initialize()
	return simulator
}

type stubConn struct {
	channels     map[string][]func([]byte)
	client       *spot.WebSocket
	books        *spot.BookManager
	postResponse []byte
	postPath     string
	postParams   json.Marshaler
	closeCount   int
}

func (stub *stubConn) Client() *spot.WebSocket { return stub.client }

func (stub *stubConn) Books() *spot.BookManager { return stub.books }

func (stub *stubConn) On(channel string, action func([]byte)) {
	if stub.channels == nil {
		stub.channels = map[string][]func([]byte){}
	}

	stub.channels[channel] = append(stub.channels[channel], action)
}

func (stub *stubConn) Write(params json.Marshaler) error { return nil }

func (stub *stubConn) Close() { stub.closeCount++ }

func (stub *stubConn) Post(path string, params json.Marshaler) ([]byte, error) {
	stub.postPath = path
	stub.postParams = params
	return stub.postResponse, nil
}

func normalizerClient() *spot.WebSocket {
	client := spot.NewWebSocket()
	client.REST.Executor = func(request *http.Request) (*http.Response, error) {
		version := request.URL.Query().Get("assetVersion")
		body := `{"error":[],"result":{}}`

		switch request.URL.Path {
		case "/0/public/Assets":
			body = `{"error":[],"result":{"XXBT":{"altname":"XBT"},"ZUSD":{"altname":"USD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC":{"altname":"XBT"},"USD":{"altname":"USD"}}}`
			}
		case "/0/public/AssetPairs":
			body = `{"error":[],"result":{"XXBTZUSD":{"wsname":"XBT/USD","base":"XXBT","quote":"ZUSD"}}}`

			if version == "1" {
				body = `{"error":[],"result":{"BTC/USD":{"wsname":"BTC/USD","base":"BTC","quote":"USD"}}}`
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}

	return client
}
