package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
		api := NewAPI(public, private, &stubConn{}, nil)

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
	newAssets := map[string]spot.AssetInfo{
		"USD": {AltName: "USD"},
	}
	oldAssets := map[string]spot.AssetInfo{
		"ZUSD": {AltName: "USD"},
	}
	newPairs := make(map[string]spot.AssetPair, 40)
	oldPairs := make(map[string]spot.AssetPair, 40)
	fees := make(map[string]kraken.TradeVolumeFees, 40)
	symbols := make([]string, 40)

	for index := range symbols {
		standard := fmt.Sprintf("ASSET-%02d", index)
		alternative := fmt.Sprintf("A%02d", index)
		legacy := fmt.Sprintf("X%02d", index)
		canonical := legacy + "ZUSD"
		symbols[index] = standard + "/USD"
		newAssets[standard] = spot.AssetInfo{AltName: alternative}
		oldAssets[legacy] = spot.AssetInfo{AltName: alternative}
		newPairs[symbols[index]] = spot.AssetPair{
			Base: standard, Quote: "USD", WSName: symbols[index],
		}
		oldPairs[canonical] = spot.AssetPair{
			Base: legacy, Quote: "ZUSD", WSName: alternative + "/USD",
		}
		fees[canonical] = kraken.TradeVolumeFees{Fee: "0.2600"}
	}

	response, err := json.Marshal(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: fees},
	})

	if err != nil {
		b.Fatal(err)
	}

	private := &stubConn{postResponse: response}
	api := NewAPI(&stubConn{}, private, &stubConn{}, nil)
	api.normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: newAssets,
		OldAssets: oldAssets,
		NewPairs:  newPairs,
		OldPairs:  oldPairs,
	})
	api.normalizerOnce.Do(func() {})
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tradeVolume, err := api.TradeVolume(symbols)

		if err != nil {
			b.Fatal(err)
		}

		if len(tradeVolume.Result.Fees) != len(symbols) {
			b.Fatal("incomplete normalized fee tier")
		}
	}
}

func TestAPIOnRoutesLevel3(t *testing.T) {
	Convey("Given a live API with stub transports", t, func() {
		viper.Set("trading.model", "live")

		public := &stubConn{}
		private := &stubConn{}
		level3 := &stubConn{}
		api := NewAPI(public, private, level3, nil)

		api.On("level3", func([]byte) {})
		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})

		Convey("Then each callback registers on its dedicated transport", func() {
			So(len(level3.channels["level3"]), ShouldEqual, 1)
			So(len(public.channels["ticker"]), ShouldEqual, 1)
			So(len(private.channels["balances"]), ShouldEqual, 1)
		})
	})

	Convey("Given a paper API with stub transports", t, func() {
		viper.Set("trading.model", "paper")

		public := &stubConn{}
		level3 := &stubConn{}
		paper := NewPaper(context.Background(), newTestSimulator())
		api := NewAPI(public, level3, level3, paper)

		api.On("level3", func([]byte) {})
		api.On("ticker", func([]byte) {})
		api.On("balances", func([]byte) {})
		api.On("executions", func([]byte) {})
		api.On("add_order", func([]byte) {})

		Convey("Then level3 registers on the shared authenticated transport", func() {
			So(len(level3.channels["level3"]), ShouldEqual, 1)
			So(len(public.channels["ticker"]), ShouldEqual, 1)
		})

		Convey("Then balances, executions, and add_order register on the paper transport instead", func() {
			So(len(level3.channels["balances"]), ShouldEqual, 0)

			_, ok := paper.sync.Load("balances")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("executions")
			So(ok, ShouldBeTrue)

			_, ok = paper.sync.Load("add_order")
			So(ok, ShouldBeTrue)
		})
	})
}

func TestAPISubscribeLevel3(t *testing.T) {
	Convey("Given a connected level3 transport with an authentication token", t, func() {
		requests := make(chan []byte, 1)
		errors := make(chan error, 1)
		upgrader := gorillawebsocket.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			connection, err := upgrader.Upgrade(writer, request, nil)

			if err != nil {
				errors <- err
				return
			}

			defer connection.Close()
			_, raw, err := connection.ReadMessage()

			if err != nil {
				errors <- err
				return
			}

			requests <- raw
			_, _, _ = connection.ReadMessage()
		}))
		defer server.Close()

		client := spot.NewWebSocket()
		client.URL = "ws" + strings.TrimPrefix(server.URL, "http")
		client.Token = "level3-token"
		So(client.Connect(), ShouldBeNil)

		level3 := &stubConn{client: client}
		api := NewAPI(&stubConn{}, &stubConn{}, level3, nil)
		viper.Set("market.l3_depth", 25)

		Convey("When level3 is subscribed", func() {
			So(api.SubscribeLevel3([]string{"BTC/USD", "ETH/USD"}), ShouldBeNil)

			select {
			case err := <-errors:
				So(err, ShouldBeNil)
			case raw := <-requests:
				request := struct {
					Method string `json:"method"`
					Params struct {
						Channel string   `json:"channel"`
						Symbols []string `json:"symbol"`
						Depth   int      `json:"depth"`
						Token   string   `json:"token"`
					} `json:"params"`
				}{}

				So(json.Unmarshal(raw, &request), ShouldBeNil)
				So(request.Method, ShouldEqual, "subscribe")
				So(request.Params.Channel, ShouldEqual, "level3")
				So(request.Params.Symbols, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
				So(request.Params.Depth, ShouldEqual, 25)
				So(request.Params.Token, ShouldEqual, "level3-token")
			case <-time.After(time.Second):
				So("level3 subscription", ShouldEqual, "received")
			}
		})

		So(client.Disconnect(), ShouldBeNil)
	})
}

func TestAPICloseSharedLevel3(t *testing.T) {
	Convey("Given a paper API sharing its authenticated and level3 transport", t, func() {
		public := &stubConn{}
		level3 := &stubConn{}
		api := NewAPI(public, level3, level3, nil)

		Convey("When the API closes", func() {
			api.Close()

			Convey("Then each physical connection closes exactly once", func() {
				So(public.closeCount, ShouldEqual, 1)
				So(level3.closeCount, ShouldEqual, 1)
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

func (stub *stubConn) Books() BookLookup { return stub.books }

func (stub *stubConn) SubscribeLevel3(symbols []string, depth int) error {
	return stub.client.SubL3(symbols, depth, map[string]any{
		"params": map[string]any{"depth": depth},
	})
}

func (stub *stubConn) On(channel string, action func([]byte)) {
	if stub.channels == nil {
		stub.channels = map[string][]func([]byte){}
	}

	stub.channels[channel] = append(stub.channels[channel], action)
}

func (stub *stubConn) Write(params json.Marshaler) error { return nil }

func (stub *stubConn) Close() { stub.closeCount++ }

func (stub *stubConn) Status() types.Status { return types.READY }

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
