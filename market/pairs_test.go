package market

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/client"
	kmarket "github.com/theapemachine/symm/kraken/market"
)

func TestNewPairs(t *testing.T) {
	convey.Convey("Given an instrument websocket snapshot", t, func() {
		testServer := newInstrumentTestServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		publicClient := client.NewPublicClient(ctx, client.WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		convey.Convey("It should connect and subscribe", func() {
			_, err := NewPairs(ctx, "EUR", publicClient)
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestPairsNames(t *testing.T) {
	convey.Convey("Given a loaded instrument snapshot", t, func() {
		testServer := newInstrumentTestServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		publicClient := client.NewPublicClient(ctx, client.WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		pairs, err := NewPairs(ctx, "EUR", publicClient)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should filter online pairs by quote", func() {
			names, err := pairs.Names(ctx)
			convey.So(err, convey.ShouldBeNil)
			convey.So(names, convey.ShouldResemble, []string{"BTC/EUR", "ETH/EUR"})
		})
	})
}

func TestPairsObserve(t *testing.T) {
	convey.Convey("Given a loaded instrument snapshot", t, func() {
		testServer := newInstrumentTestServer(t)
		defer testServer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		publicClient := client.NewPublicClient(ctx, client.WithWebSocketURL(testServer.url))
		convey.So(publicClient.Connect(), convey.ShouldBeNil)
		defer publicClient.Close()

		pairs, err := NewPairs(ctx, "EUR", publicClient)
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("It should expose pairs through Observe", func() {
			_, err := pairs.GetAll(ctx)
			convey.So(err, convey.ShouldBeNil)

			observation, err := pairs.Observe(ctx)
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(observation.Pairs), convey.ShouldEqual, 2)
			convey.So(observation.Pairs[0].Wsname, convey.ShouldEqual, "BTC/EUR")
		})
	})
}

type instrumentTestServer struct {
	server *httptest.Server
	url    string
}

func newInstrumentTestServer(t *testing.T) *instrumentTestServer {
	t.Helper()

	upgrader := websocket.Upgrader{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			for _, response := range instrumentTestResponses(payload) {
				if err := conn.WriteMessage(websocket.TextMessage, response); err != nil {
					return
				}
			}
		}
	})

	server := httptest.NewServer(handler)

	return &instrumentTestServer{
		server: server,
		url:    strings.Replace(server.URL, "http://", "ws://", 1),
	}
}

func (testServer *instrumentTestServer) Close() {
	testServer.server.Close()
}

func instrumentTestResponses(payload []byte) [][]byte {
	var frame struct {
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}

	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil
	}

	if frame.Method != "subscribe" {
		return nil
	}

	channel, _ := frame.Params["channel"].(string)
	ack, _ := json.Marshal(map[string]any{
		"method":  "subscribe",
		"success": true,
		"result": map[string]any{
			"channel":  channel,
			"snapshot": true,
		},
	})

	if channel != "instrument" {
		return [][]byte{ack}
	}

	snapshot, _ := json.Marshal(kmarket.InstrumentMessage{
		Channel: "instrument",
		Type:    kmarket.InstrumentUpdateTypeSnapshot,
		Data: kmarket.InstrumentData{
			Pairs: []kmarket.Instrument{
				{
					Symbol:  "BTC/EUR",
					Base:    "BTC",
					Quote:   "EUR",
					Status:  kmarket.PairStatusOnline,
					CostMin: 0.45,
				},
				{
					Symbol:  "ETH/EUR",
					Base:    "ETH",
					Quote:   "EUR",
					Status:  kmarket.PairStatusOnline,
					CostMin: 0.43,
				},
				{
					Symbol:  "BTC/USD",
					Base:    "BTC",
					Quote:   "USD",
					Status:  kmarket.PairStatusOnline,
					CostMin: 0.5,
				},
			},
		},
	})

	return [][]byte{ack, snapshot}
}
