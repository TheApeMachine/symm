package transport_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGenericPrimitives(t *testing.T) {
	Convey("Given topology primitives", t, func() {
		Convey("Fork and Join", func() {
			branchA := types.Value[int, int](func(x int) int { return x + 10 })
			branchB := types.Value[int, int](func(x int) int { return x * 2 })
			fork := transport.NewFork(branchA, branchB)

			branches := fork(5)
			So(branches, ShouldResemble, []int{15, 10})

			join := transport.NewJoin(func(vals []int) int {
				return vals[0] + vals[1]
			})
			combined := join(branches)
			So(combined, ShouldEqual, 25)
		})

		Convey("Route and Gate", func() {
			routes := map[string]types.Value[int, string]{
				"even": func(x int) string { return "is_even" },
				"odd":  func(x int) string { return "is_odd" },
			}
			router := transport.NewRoute(func(x int) string {
				if x%2 == 0 {
					return "even"
				}
				return "odd"
			}, routes)

			So(router(4), ShouldEqual, "is_even")
			So(router(7), ShouldEqual, "is_odd")

			gate := transport.NewGate(func(x int) bool { return x > 10 })
			So(gate(5), ShouldBeNil)
			val := gate(15)
			So(val, ShouldNotBeNil)
			So(*val, ShouldEqual, 15)
		})
	})

	Convey("Given auth and signing primitives", t, func() {
		nonceGen := transport.NewNonce()
		n1 := nonceGen(nil)
		n2 := nonceGen(nil)
		So(n2, ShouldBeGreaterThan, n1)

		tsGen := transport.NewTimestamp()
		So(tsGen(nil), ShouldBeGreaterThan, 0)

		sha := transport.NewSHA256()
		h := sha([]byte("test"))
		So(len(h), ShouldEqual, 32)

		hmac := transport.NewHMACSHA512([]byte("secret"))
		sig := hmac([]byte("payload"))
		So(len(sig), ShouldEqual, 64)

		signer := transport.NewSigner("my-key", "my-secret")
		req := &transport.HTTPRequest{
			Method: "POST",
			URL:    "https://api.kraken.com/0/private/OpenOrders",
			Body:   []byte(`{"nonce": 123}`),
		}
		signed := signer(req)
		So(signed.Headers["API-Key"], ShouldEqual, "my-key")
		So(signed.Headers["API-Sign"], ShouldNotBeBlank)
	})

	Convey("Given HTTP execution primitives", t, func() {
		client := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"ok"}`))),
				}, nil
			}),
		}

		reqBuilder := transport.NewHTTPRequest("GET", "http://example.com/api")
		req := reqBuilder(nil)
		paramSetter := transport.NewHTTPParam("pair", "BTCUSD")
		req = paramSetter(req)

		executor := transport.NewHTTPExecute(client)
		resp := executor(req)
		So(resp, ShouldNotBeNil)
		So(resp.StatusCode, ShouldEqual, http.StatusOK)

		extractor := transport.NewResponseExtract()
		body := extractor(resp)
		So(string(body), ShouldEqual, `{"status":"ok"}`)
	})

	Convey("Given JSON encoding primitives", t, func() {
		type Payload struct {
			Symbol string  `json:"symbol"`
			Price  float64 `json:"price"`
		}

		encoder := transport.NewJSONEncode[Payload]()
		decoder := transport.NewJSONDecode[Payload]()

		data := encoder(Payload{Symbol: "ETH/USD", Price: 3000.0})
		So(len(data), ShouldBeGreaterThan, 0)

		decoded := decoder(data)
		So(decoded.Symbol, ShouldEqual, "ETH/USD")
		So(decoded.Price, ShouldEqual, 3000.0)
	})

	Convey("Given WebSocket primitives", t, func() {
		subGen := transport.NewSubscription("subscribe", "trade", "BTC/USD")
		subMsg := subGen(nil)
		So(subMsg["method"], ShouldEqual, "subscribe")

		pingPong := transport.NewPingPong()
		pingMsg := &transport.WSMessage{Type: 9, Payload: []byte("ping")} // 9 is PingMessage
		pongMsg := pingPong(pingMsg)
		So(pongMsg.Type, ShouldEqual, 10) // 10 is PongMessage

		_ = transport.NewWSConnect("ws://invalid.test")(context.Background())
	})
}
