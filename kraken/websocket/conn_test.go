package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	level3fixture "github.com/theapemachine/symm/tests/fixtures/level3"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
	"github.com/theapemachine/symm/types"
)

func TestAPIStatus(t *testing.T) {
	Convey("Given an API lifecycle", t, func() {
		public := &stubConn{client: normalizerClient()}
		api := NewAPI(context.Background(), public, &stubConn{}, nil)

		Convey("It should report the state set by initialization", func() {
			So(api.Status(), ShouldEqual, types.INITIALIZING)
			So(api.Initialize(), ShouldBeNil)
			So(api.Status(), ShouldEqual, types.READY)
		})
	})
}

func TestAPISubscribeInstruments(t *testing.T) {
	Convey("Given an injected public connection", t, func() {
		public := &stubConn{}
		api := &API{public: public}
		So(api.SubscribeInstruments(), ShouldBeNil)
		So(public.writes, ShouldHaveLength, 1)
		So(string(public.writes[0]), ShouldContainSubstring, `"channel":"instrument"`)
	})
}

func TestAPISubscribeTicker(t *testing.T) {
	Convey("Given an injected public connection", t, func() {
		public := &stubConn{}
		api := &API{public: public}
		So(api.SubscribeTicker([]string{"BTC/USD"}), ShouldBeNil)
		So(public.writes, ShouldHaveLength, 1)
		So(string(public.writes[0]), ShouldContainSubstring, `"channel":"ticker"`)
		So(string(public.writes[0]), ShouldContainSubstring, `"BTC/USD"`)
	})
}

func TestAPISubscribeTrade(t *testing.T) {
	Convey("Given an injected public connection", t, func() {
		public := &stubConn{}
		api := &API{public: public}
		So(api.SubscribeTrade([]string{"BTC/USD"}), ShouldBeNil)
		So(public.writes, ShouldHaveLength, 1)
		request := map[string]any{}
		So(json.Unmarshal(public.writes[0], &request), ShouldBeNil)

		params := request["params"].(map[string]any)

		Convey("It should send the snapshot request through Conn", func() {
			So(params["channel"], ShouldEqual, "trade")
			So(params["symbol"], ShouldResemble, []any{"BTC/USD"})
			So(params["snapshot"], ShouldEqual, true)
		})
	})
}

func TestAPISubscribeBook(t *testing.T) {
	previousDepth := viper.Get("market.book.depth")
	t.Cleanup(func() { viper.Set("market.book.depth", previousDepth) })
	viper.Set("market.book.depth", 25)

	Convey("Given an injected public connection", t, func() {
		public := &stubConn{}
		api := &API{public: public}
		So(api.SubscribeBook([]string{"BTC/USD"}), ShouldBeNil)
		So(public.writes, ShouldHaveLength, 1)
		request := map[string]any{}
		So(json.Unmarshal(public.writes[0], &request), ShouldBeNil)
		params := request["params"].(map[string]any)
		So(params["channel"], ShouldEqual, "book")
		So(params["symbol"], ShouldResemble, []any{"BTC/USD"})
		So(params["depth"], ShouldEqual, float64(25))
	})
}

/*
TestAPIResubscribeMarket proves reconnect re-issues instrument plus the three
market channels for the cached symbol universe without a parallel intent cache.
*/
func TestAPIResubscribeMarket(t *testing.T) {
	previousDepth := viper.Get("market.book.depth")
	previousBatch := viper.Get("market.subscribe_batch")
	t.Cleanup(func() {
		viper.Set("market.book.depth", previousDepth)
		viper.Set("market.subscribe_batch", previousBatch)
	})
	viper.Set("market.book.depth", 10)
	viper.Set("market.subscribe_batch", 10)

	Convey("Given symbols previously selected by Instrument", t, func() {
		public := &stubConn{}
		api := &API{public: public}
		So(api.ResubscribeMarket([]string{"BTC/USD", "ETH/USD"}), ShouldBeNil)

		Convey("It should write instrument then trade, book, and ticker", func() {
			So(public.writes, ShouldHaveLength, 4)
			So(string(public.writes[0]), ShouldContainSubstring, `"channel":"instrument"`)
			So(string(public.writes[1]), ShouldContainSubstring, `"channel":"trade"`)
			So(string(public.writes[2]), ShouldContainSubstring, `"channel":"book"`)
			So(string(public.writes[3]), ShouldContainSubstring, `"channel":"ticker"`)
			So(string(public.writes[1]), ShouldContainSubstring, `"BTC/USD"`)
			So(string(public.writes[1]), ShouldContainSubstring, `"ETH/USD"`)
		})
	})
}

func TestAPISubscribeBalance(t *testing.T) {
	Convey("Given an injected authenticated connection", t, func() {
		client := spot.NewWebSocket()
		client.Token = "private-token"
		private := &stubConn{client: client}
		api := &API{private: private, live: true}
		So(api.SubscribeBalance(), ShouldBeNil)
		So(private.writes, ShouldHaveLength, 1)
		So(string(private.writes[0]), ShouldContainSubstring, `"channel":"balances"`)
		So(string(private.writes[0]), ShouldContainSubstring, `"token":"private-token"`)
	})
}

func TestAPISubscribeExecutions(t *testing.T) {
	Convey("Given an injected authenticated connection", t, func() {
		client := spot.NewWebSocket()
		client.Token = "private-token"
		private := &stubConn{client: client}
		api := &API{private: private, live: true}
		So(api.SubscribeExecutions(), ShouldBeNil)
		So(private.writes, ShouldHaveLength, 1)
		So(string(private.writes[0]), ShouldContainSubstring, `"channel":"executions"`)
		So(string(private.writes[0]), ShouldContainSubstring, `"snap_orders":true`)
		So(string(private.writes[0]), ShouldContainSubstring, `"snap_trades":true`)
	})
}

func TestAPIBooks(t *testing.T) {
	Convey("Given an SDK book managed by a level3 transport", t, func() {
		manager := spot.NewBookManager()
		managed := manager.CreateBook("BTC/USD", 10)
		managed.Update(&book.UpdateOptions{
			Direction: book.Ask, ID: "ask",
			Price: decimal.NewFromFloat64(101), Quantity: decimal.NewFromFloat64(1),
			Timestamp: time.Unix(1, 0),
		})
		live := &Live{books: manager}
		api := &API{level3: NewLevel3Registry()}
		api.level3.Attach("BTC/USD", live)

		Convey("It should expose that same SDK manager directly", func() {
			for books := range api.Books() {
				So(books, ShouldEqual, manager)
				So(books.GetBook("BTC/USD"), ShouldEqual, managed)
			}
		})

		Convey("It should protect PeekBook reads during websocket updates", func() {
			live.isLevel3 = true
			checksum := managed.L3Checksum("").LocalChecksum
			raw := fmt.Appendf(nil, `{
				"channel":"level3",
				"type":"update",
				"data":[{
					"symbol":"BTC/USD",
					"checksum":%s,
					"bids":[],
					"asks":[{
						"event":"modify",
						"order_id":"ask",
						"limit_price":101,
						"order_qty":1,
						"timestamp":"1970-01-01T00:00:01Z"
					}]
				}]
			}`, checksum)
			event := &callback.Event[*sdkkraken.WebSocketMessage]{
				Data: sdkkraken.NewWebSocketMessage(raw),
			}
			start := make(chan struct{})
			failures := make(chan error, 8)
			wait := sync.WaitGroup{}
			wait.Add(2)

			go func() {
				defer wait.Done()
				<-start

				for range 256 {
					if err := live.updateLevel3(event); err != nil {
						failures <- err
						return
					}
				}
			}()

			go func() {
				defer wait.Done()
				<-start

				for range 256 {
					var readErr error

					ok := api.PeekBook("BTC/USD", func(symbolBook *book.Book) {
						ask := symbolBook.BestAsk()

						if ask == nil || len(ask.Queue()) != 1 {
							readErr = fmt.Errorf("incomplete SDK book read")
							return
						}

						// Range the live Levels map — the crash path without a lease.
						for _, level := range symbolBook.Asks.Levels {
							if level == nil {
								readErr = fmt.Errorf("nil ask level")
								return
							}
						}
					})

					if !ok {
						failures <- fmt.Errorf("PeekBook missed BTC/USD")
						return
					}

					if readErr != nil {
						failures <- readErr
						return
					}
				}
			}()

			close(start)
			wait.Wait()
			close(failures)

			for err := range failures {
				So(err, ShouldBeNil)
			}
		})
	})
}

/*
TestAPIApplyLevel3Peekable verifies the injected apply path stays synchronous:
a book fed through ApplyLevel3 is immediately peekable once Apply returns, so
package tests never race the async reader worker before PeekBook.
*/
func TestAPIApplyLevel3Peekable(t *testing.T) {
	Convey("Given an L3 transport attached to an API", t, func() {
		live := New(context.Background(), nil, true, Level3WebSocketURL)
		live.client.Reconnect = func() {}
		defer live.Close()

		api := &API{level3: NewLevel3Registry()}
		api.level3.Attach("BTC/USD", live)

		Convey("Then no book is peekable before any apply", func() {
			So(api.PeekBook("BTC/USD", func(*book.Book) {}), ShouldBeFalse)
		})

		// Seed the scratch book exactly as UpdateL3 parses the frame (decimals
		// from their string form) so the derived checksum matches the applied
		// book — NewFromInt64 renders fixed-point trailing digits and would not.
		bidPrice, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)
		bidQuantity, err := decimal.NewFromString("1")
		So(err, ShouldBeNil)

		scratch := spot.NewBookManager()
		scratchBook := scratch.CreateBook("BTC/USD", 10)
		scratchBook.EnableMaxDepth = false
		scratchBook.NoBookCrossing = false
		scratchBook.Update(&book.UpdateOptions{
			Direction: book.Bid,
			ID:        "bid-1",
			Price:     bidPrice,
			Quantity:  bidQuantity,
			Timestamp: time.Unix(1, 0),
		})
		checksum := scratchBook.L3Checksum("").LocalChecksum

		So(live.ApplyLevel3([]byte(
			`{"method":"subscribe","params":{"channel":"level3","symbol":["BTC/USD"],"depth":10}}`,
		)), ShouldBeNil)

		update := fmt.Appendf(nil, `{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"checksum":%s,
				"bids":[{"event":"add","order_id":"bid-1","limit_price":100,"order_qty":1,"timestamp":"1970-01-01T00:00:01Z"}],
				"asks":[]
			}]
		}`, checksum)

		So(live.ApplyLevel3(update), ShouldBeNil)

		Convey("Then the book is peekable synchronously after Apply", func() {
			best := 0.0
			ok := api.PeekBook("BTC/USD", func(symbolBook *book.Book) {
				if bid := symbolBook.BestBid(); bid != nil {
					best = bid.Price.Float64()
				}
			})

			So(ok, ShouldBeTrue)
			So(best, ShouldEqual, 100.0)
		})
	})
}

/*
TestAPIInjectLevel3 proves generated Kraken snapshots and updates preserve the
production Level3 checksum chain across consecutive market states.
*/
func TestAPIInjectLevel3(t *testing.T) {
	Convey("Given a fixture-driven Level3 connection", t, func() {
		symbols := []string{"SIM1/USD"}
		signal := marketsignal.New(symbols)
		fixture := level3fixture.NewMarket(symbols, signal)
		conn := newStubConn()
		viper.Set("market.l3_depth", 10)
		viper.Set("system.actor.buffer", 64)
		api := NewAPI(context.Background(), conn, conn, nil)
		api.InjectLevel3(conn.Actor, conn, symbols)
		Reset(func() {
			api.Close()
			conn.Close()
		})
		signal.Bootstrap()

		for payload := range fixture.Generate() {
			conn.level3.Send(payload)
		}

		time.Sleep(50 * time.Millisecond)

		quote, exists := signal.Quote(symbols[0])
		So(exists, ShouldBeTrue)
		So(signal.Apply(marketsignal.Step{
			Advance: time.Second,
			Actions: []marketsignal.Action{{
				Kind:   marketsignal.Trade,
				Symbol: symbols[0],
				Side:   "buy",
				Qty:    quote.AskQty + 5,
			}},
		}), ShouldBeNil)
		var update []byte

		for payload := range fixture.Generate() {
			update = payload
			conn.level3.Send(payload)
		}

		time.Sleep(50 * time.Millisecond)

		Convey("A complete touch and second-level fill should reach the production ledger", func() {
			So(conn.reported, ShouldBeNil)
			best := 0.0
			So(api.PeekBook(symbols[0], func(symbolBook *book.Book) {
				ask := symbolBook.BestAsk()

				if ask == nil || ask.Price == nil {
					return
				}

				best = ask.Price.Float64()
			}), ShouldBeTrue)
			So(best, ShouldBeGreaterThan, 0)
			So(best, ShouldEqual, quote.Ask+marketsignal.PriceIncrement)
		})

		Convey("A production ledger rejection should reach the connection error sink", func() {
			var corrupted map[string]any
			So(json.Unmarshal(update, &corrupted), ShouldBeNil)
			corrupted["data"].([]any)[0].(map[string]any)["checksum"] = 1
			payload, err := json.Marshal(corrupted)
			So(err, ShouldBeNil)
			conn.level3.Send(payload)
			time.Sleep(50 * time.Millisecond)
			So(conn.reported, ShouldNotBeNil)
		})
	})
}

/*
BenchmarkAPIPeekBook measures leased access to one SDK-managed book.
*/
func BenchmarkAPIPeekBook(b *testing.B) {
	manager := spot.NewBookManager()
	manager.CreateBook("BTC/USD", 10)
	live := &Live{books: manager}
	api := &API{level3: NewLevel3Registry()}
	api.level3.Attach("BTC/USD", live)
	b.ReportAllocs()

	for b.Loop() {
		if !api.PeekBook("BTC/USD", func(symbolBook *book.Book) {
			if symbolBook == nil {
				b.Fatal("managed book missing")
			}
		}) {
			b.Fatal("PeekBook missed BTC/USD")
		}
	}
}

func TestAPITradeVolume(t *testing.T) {
	Convey("Given a private TradeVolume response keyed by the requested pair", t, func() {
		viper.Set("trading.model", "live")
		public := &stubConn{client: normalizerClient()}
		private := &stubConn{client: normalizerClient(), postResponse: []byte(`{
			"error":[],
			"result":{
				"fees":{"XXBTZUSD":{"fee":"0.2600"},"AUSD":{"fee":"0.4000"}},
				"fees_maker":{"XXBTZUSD":{"fee":"0.1600"},"AUSD":{"fee":"0.2500"}}
			}
		}`)}
		api := NewAPI(context.Background(), public, private, nil)
		So(api.Initialize(), ShouldBeNil)

		Convey("When the fee tier is requested", func() {
			tradeVolume, err := api.TradeVolume([]string{"BTC/USD", "A/USD"})

			Convey("Then the private endpoint is used and SDK pair names are normalized", func() {
				So(err, ShouldBeNil)
				So(private.postPath, ShouldEqual, TradeVolumeEndpoint)
				encoded, encodeErr := private.postParams.MarshalJSON()
				So(encodeErr, ShouldBeNil)
				So(string(encoded), ShouldContainSubstring, `"pair":"BTC/USD,A/USD"`)
				So(string(encoded), ShouldContainSubstring, `"fee_schedule":true`)
				So(tradeVolume.Fees["BTC/USD"].Fee.Float64(), ShouldEqual, 0.26)
				So(tradeVolume.FeesMaker["BTC/USD"].Fee.Float64(), ShouldEqual, 0.16)
				So(tradeVolume.Fees["A/USD"].Fee.Float64(), ShouldEqual, 0.40)
				So(tradeVolume.FeesMaker["A/USD"].Fee.Float64(), ShouldEqual, 0.25)
			})
		})
	})
}

func BenchmarkAPITradeVolume(b *testing.B) {
	fees := make(map[string]kraken.TradeVolumeFee, 40)
	symbols := make([]string, 40)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ASSET-%02d/USD", index)
		fees[symbols[index]] = kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}
	}

	response, err := json.Marshal(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: fees},
	})

	if err != nil {
		b.Fatal(err)
	}

	private := &stubConn{client: normalizerClient(), postResponse: response}
	api := NewAPI(
		context.Background(),
		&stubConn{client: normalizerClient()},
		private,
		nil,
	)

	if err := api.Initialize(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		tradeVolume, err := api.TradeVolume(symbols)

		if err != nil {
			b.Fatal(err)
		}

		if tradeVolume != nil && len(tradeVolume.Fees) != len(symbols) {
			b.Fatal("incomplete fee tier")
		}
	}
}

/*
TestAPILevel3BatchSize verifies that L3 connection batches honor Kraken's
depth-weighted subscription-rate budget instead of counting symbols directly.
*/
func TestAPILevel3BatchSize(t *testing.T) {
	previousDepth := viper.Get("market.l3_depth")
	previousLimit := viper.Get("market.l3_rate_limit")
	t.Cleanup(func() {
		viper.Set("market.l3_depth", previousDepth)
		viper.Set("market.l3_rate_limit", previousLimit)
	})

	Convey("Given Kraken's standard L3 subscription budget", t, func() {
		viper.Set("market.l3_rate_limit", 200)

		Convey("Depth 10 admits forty symbols per connection", func() {
			viper.Set("market.l3_depth", 10)
			batchSize, err := level3BatchSize()
			So(err, ShouldBeNil)
			So(batchSize, ShouldEqual, 40)
		})

		Convey("Depth 100 admits eight symbols per connection", func() {
			viper.Set("market.l3_depth", 100)
			batchSize, err := level3BatchSize()
			So(err, ShouldBeNil)
			So(batchSize, ShouldEqual, 8)
		})

		Convey("Depth 1000 admits two symbols per connection", func() {
			viper.Set("market.l3_depth", 1000)
			batchSize, err := level3BatchSize()
			So(err, ShouldBeNil)
			So(batchSize, ShouldEqual, 2)
		})
	})
}

/*
BenchmarkAPILevel3BatchSize measures the depth-weighted L3 batch calculation
used whenever the market universe is subscribed.
*/
func BenchmarkAPILevel3BatchSize(b *testing.B) {
	viper.Set("market.l3_depth", 10)
	viper.Set("market.l3_rate_limit", 200)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := level3BatchSize(); err != nil {
			b.Fatal(err)
		}
	}
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

type stubConn struct {
	*types.Actor
	level3       *types.Subscription[any]
	client       *spot.WebSocket
	writes       [][]byte
	postResponse []byte
	postPath     string
	postParams   json.Marshaler
	closeCount   int
	reported     error
}

func newStubConn() *stubConn {
	stub := &stubConn{
		level3: &types.Subscription[any]{
			Channel: make(chan any, 64),
		},
	}
	stub.Actor = types.NewActor(context.Background(), "stub", nil)
	stub.AddRoot("level3", stub.level3)
	// Match MockConn: root fan-out only runs after Start, so InjectLevel3's
	// subscriber actually receives frames sent on the level3 root.
	stub.Start()

	return stub
}

func (stub *stubConn) Report(err error) { stub.reported = err }

func (stub *stubConn) Client() *spot.WebSocket { return stub.client }

func (stub *stubConn) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return err
	}

	stub.writes = append(stub.writes, raw)

	return nil
}

func (stub *stubConn) Close() {
	stub.closeCount++

	if stub.Actor != nil {
		_ = stub.Actor.Close()
	}
}

func (stub *stubConn) Root() *types.Actor { return stub.Actor }

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
