package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGenericPrimitives(t *testing.T) {
	Convey("Given topology primitives", t, func() {
		Convey("Fork and Join", func() {
			branchA := types.Value[int, int](func(x int) int { return x + 10 })
			branchB := types.Value[int, int](func(x int) int { return x * 2 })
			fork := transport.NewFork(branchA, branchB)

			branches := fork(5)
			So(branches, ShouldResemble, []int{15, 10})

			join := transport.NewJoin(types.Value[[]int, int](func(vals []int) int {
				return vals[0] + vals[1]
			}))
			combined := join(branches)
			So(combined, ShouldEqual, 25)
		})

		Convey("Route and Gate", func() {
			routes := map[string]types.Value[int, string]{
				"even": func(x int) string { return "is_even" },
				"odd":  func(x int) string { return "is_odd" },
			}
			router := transport.NewRoute(types.Value[int, string](func(x int) string {
				if x%2 == 0 {
					return "even"
				}
				return "odd"
			}), routes)

			So(router(4), ShouldEqual, "is_even")
			So(router(7), ShouldEqual, "is_odd")

			gate := transport.NewGate(types.Value[int, bool](func(x int) bool { return x > 10 }))
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

		hmac := transport.NewHMACSHA512(types.Const([]byte("secret")))
		sig := hmac([]byte("payload"))
		So(len(sig), ShouldEqual, 64)

		b64Enc := transport.NewBase64Encode()
		b64Dec := transport.NewBase64Decode()
		encoded := b64Enc([]byte("hello world"))
		So(encoded, ShouldEqual, "aGVsbG8gd29ybGQ=")
		decoded := b64Dec(encoded)
		So(string(decoded), ShouldEqual, "hello world")

		headerAuth := transport.NewHeaderAuth(types.Const("API-Key"), types.Const("my-key"))
		data := headerAuth(map[string]any{"action": "ping"})
		headers, ok := data["headers"].(map[string]string)
		So(ok, ShouldBeTrue)
		So(headers["API-Key"], ShouldEqual, "my-key")

		bearerAuth := transport.NewBearerAuth(types.Const("my-token"))
		data = bearerAuth(data)
		headers = data["headers"].(map[string]string)
		So(headers["Authorization"], ShouldEqual, "Bearer my-token")
	})

	Convey("Given HTTP execution primitives", t, func() {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","count":42}`))
		}))
		defer ts.Close()

		httpReq := transport.NewHTTPRequest(
			types.Const("GET"),
			types.Const(ts.URL),
		)

		res := httpReq(nil)
		So(res, ShouldNotBeNil)
		So(res["status"], ShouldEqual, "ok")
		So(res["status_code"], ShouldEqual, 200)
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
		msgGen := transport.NewJSONMessage(types.Const[any](map[string]any{
			"channel": "trade",
			"symbol":  []string{"BTC/USD"},
		}))
		msg := msgGen(nil)
		So(msg, ShouldNotBeNil)

		batcher := transport.NewBatch[string](types.Const(2))
		batches := batcher([]string{"A", "B", "C", "D", "E"})
		So(len(batches), ShouldEqual, 3)
		So(batches[0], ShouldResemble, []string{"A", "B"})

		connect := transport.NewWSConnect(types.Const("ws://invalid.test.nowhere:9999"))
		conn := connect(context.Background())
		So(conn, ShouldBeNil)

		proc := transport.NewProcess(types.Const("echo"))
		out := proc([]string{`{"balances":{"USD":{"available":100}}}`})
		decode := transport.NewDecodeJSON()
		res := decode(out)
		So(res, ShouldNotBeNil)
	})
}
