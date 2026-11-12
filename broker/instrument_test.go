package broker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bytedance/sonic"
	gorilla "github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
seedClient fails the first Read (soft disconnect), then yields a snapshot after
resubscribe — mirrors Kraken 1013 during instrument seed.
*/
type seedClient struct {
	reads atomic.Int32
	frame []byte
}

func (client *seedClient) Read(...string) ([]byte, error) {
	if client.reads.Add(1) == 1 {
		return nil, errors.New("websocket: close 1013: Market data unavailable")
	}

	return client.frame, nil
}

func (client *seedClient) Write([]byte) error { return nil }

func (client *seedClient) Post(string, any) ([]byte, error) { return nil, nil }

func (client *seedClient) Watch(string, any, json.Unmarshaler) {}

func TestSeedSnapshotRetriesSoftDisconnect(t *testing.T) {
	Convey("Soft disconnect during seed re-subscribes and still arms", t, func() {
		costMin, _ := decimal.NewFromString("1.0")
		snapshot := &kraken.Instrument{
			Channel: "instrument",
			Type:    "snapshot",
		}
		snapshot.Data.Pairs = []kraken.InstrumentPair{{
			Symbol:  "BTC/USD",
			Base:    "BTC",
			Quote:   "USD",
			Status:  "online",
			CostMin: &costMin,
		}}

		frame, err := sonic.Marshal(snapshot)
		So(err, ShouldBeNil)

		public := &seedClient{frame: frame}
		instrument := &Instrument{
			System: runtime.NewSystem(context.Background(), "instrument"),
			public: public,
			cache:  &sync.Map{},
			quote:  "USD",
		}

		So(instrument.seedSnapshot([]byte(`{"method":"subscribe"}`)), ShouldBeNil)
		So(public.reads.Load(), ShouldEqual, 2)
		So(instrument.Has("BTC/USD"), ShouldBeTrue)
	})
}

func TestSubscribeRequiresLevel3Reader(t *testing.T) {
	Convey("Given an instrument without a Level3 reader", t, func() {
		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}

		Convey("SubscribeBooks refuses before opening any socket", func() {
			So(instrument.SubscribeBooks(Level3{
				Stale: func([]string) {},
			}), ShouldNotBeNil)
		})
	})
}

func TestLevel3FrameSize(t *testing.T) {
	Convey("Given Kraken's standard level3 subscription-rate budget", t, func() {
		Convey("A frame carries only as many symbols as the budget pays for", func() {
			size, err := level3FrameSize(100, 200)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 8)

			size, err = level3FrameSize(10, 200)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 40)
		})

		Convey("An unsupported depth is an error", func() {
			_, err := level3FrameSize(25, 200)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestInstrumentCacheFilters(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Cache keeps only online quote pairs", func() {
			instrument := &Instrument{
				cache:    &sync.Map{},
				quote:    "USD",
				excluded: []string{"BAD"},
			}
			okMin, _ := decimal.NewFromString("1")

			instrument.Cache([]kraken.InstrumentPair{
				{Symbol: "OK/USD", Base: "OK", Quote: "USD", Status: "online", CostMin: &okMin},
				{Symbol: "BAD/USD", Base: "BAD", Quote: "USD", Status: "online", CostMin: &okMin},
				{Symbol: "OFF/USD", Base: "OFF", Quote: "USD", Status: "cancel_only", CostMin: &okMin},
				{Symbol: "OK/EUR", Base: "OK", Quote: "EUR", Status: "online", CostMin: &okMin},
			})

			So(instrument.Has("OK/USD"), ShouldBeTrue)
			So(instrument.Has("BAD/USD"), ShouldBeFalse)
			So(instrument.Has("OFF/USD"), ShouldBeFalse)

			pairs := instrument.Pairs()
			So(len(pairs), ShouldEqual, 1)
			So(pairs[0].Symbol, ShouldEqual, "OK/USD")
		})

		Convey("NormalizerFor tolerates nil instrument", func() {
			So(NormalizerFor(nil), ShouldNotBeNil)
		})

		Convey("NormalizerFor honors QtyIncrement lot steps", func() {
			step := decimal.RequireFromString("0.01")
			instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
			instrument.cache.Store("LOT/USD", kraken.InstrumentPair{
				Symbol: "LOT/USD", Base: "LOT", Quote: "USD",
				Status: "online", QtyPrecision: 8, QtyIncrement: &step,
			})

			normalizer := NormalizerFor(instrument)
			raw := decimal.RequireFromString("1.234567")
			sdkQty, err := toSDK(&raw)
			So(err, ShouldBeNil)

			sized, err := normalizer.FormatSize("LOT/USD", sdkQty)
			So(err, ShouldBeNil)
			So(sized.String(), ShouldEqual, "1.23")
		})
	})
}

func TestSpotOpen(t *testing.T) {
	Convey("SpotOpen tracks Kraken online status in the cache", t, func() {
		instrument := &Instrument{cache: &sync.Map{}}

		instrument.cache.Store("ON/USD", kraken.InstrumentPair{
			Symbol: "ON/USD", Status: "online",
		})
		instrument.cache.Store("OFF/USD", kraken.InstrumentPair{
			Symbol: "OFF/USD", Status: "cancel_only",
		})

		So(instrument.SpotOpen("ON/USD"), ShouldBeTrue)
		So(instrument.SpotOpen("OFF/USD"), ShouldBeFalse)
		So(instrument.SpotOpen("MISSING/USD"), ShouldBeFalse)
		So(instrument.SpotOpen(""), ShouldBeFalse)
	})
}

type tradeSubClient struct {
	writes int
	hook   func() error
}

func (client *tradeSubClient) Read(...string) ([]byte, error) { return nil, nil }

func (client *tradeSubClient) Write(buf []byte) error {
	client.writes++
	return nil
}

func (client *tradeSubClient) Post(string, any) ([]byte, error) { return nil, nil }

func (client *tradeSubClient) Watch(string, any, json.Unmarshaler) {}

func (client *tradeSubClient) OnReconnect(fn func() error) {
	client.hook = fn
}

func TestInstrumentLifecycle(t *testing.T) {
	Convey("NewInstrument arms from the public snapshot", t, func() {
		costMin := decimal.RequireFromString("1")
		snapshot := &kraken.Instrument{
			Channel: "instrument",
			Type:    "snapshot",
		}
		snapshot.Data.Pairs = []kraken.InstrumentPair{{
			Symbol: "NEW/USD", Base: "NEW", Quote: "USD",
			Status: "online", CostMin: &costMin,
		}}

		frame, err := json.Marshal(snapshot)
		So(err, ShouldBeNil)

		public := &seedClient{frame: frame}
		instrument := NewInstrument(context.Background(), public)

		So(instrument, ShouldNotBeNil)
		So(instrument.Has("NEW/USD"), ShouldBeTrue)
		So(instrument.Status(), ShouldEqual, runtime.READY)
	})

	Convey("Level3 and trade subscriptions write framed requests", t, func() {
		auth := kraken.NewAuth()
		auth.SeedToken("l3-token")

		var l3Writes int
		l3Write := func(buf []byte) error {
			l3Writes++
			return nil
		}

		instrument := &Instrument{
			System:  runtime.NewSystem(context.Background(), "instrument"),
			auth:    auth,
			Level3:  &sync.Map{},
			symbols: []string{"L3/USD", "TR/USD"},
			cache:   &sync.Map{},
			quote:   "USD",
		}

		So(instrument.SubscribeLevel3(l3Write, []string{"L3/USD"}), ShouldBeNil)
		So(l3Writes, ShouldBeGreaterThan, 0)

		l3Writes = 0
		So(instrument.UnsubscribeLevel3(l3Write, []string{"L3/USD"}), ShouldBeNil)
		So(l3Writes, ShouldEqual, 1)

		So(instrument.ResubscribeLevel3(""), ShouldBeNil)
		So(instrument.ResubscribeLevel3("NOPE/USD"), ShouldBeNil)

		public := &countingClient{writes: new(int)}
		instrument.public = public
		instrument.symbols = []string{"TR/USD", "ETH/USD"}

		So(instrument.SubscribeTrades(), ShouldBeNil)
		So(instrument.Status(), ShouldEqual, runtime.READY)

		So(instrument.Unsubscribe(), ShouldBeNil)
		So(instrument.Status(), ShouldEqual, runtime.WAITING)
	})

	Convey("SubscribeTrades registers a public reconnect hook", t, func() {
		public := &tradeSubClient{}

		instrument := &Instrument{
			System:  runtime.NewSystem(context.Background(), "instrument"),
			public:  public,
			symbols: []string{"RC/USD"},
		}

		So(instrument.SubscribeTrades(), ShouldBeNil)
		So(public.hook, ShouldNotBeNil)
		So(public.hook(), ShouldBeNil)
		So(public.writes, ShouldBeGreaterThan, 0)
	})
}

func TestInstrumentSubscribeBooks(t *testing.T) {
	Convey("SubscribeBooks requires level3 reader and stale hooks", t, func() {
		instrument := &Instrument{
			System:  runtime.NewSystem(context.Background(), "instrument"),
			symbols: []string{"BK/USD"},
		}

		err := instrument.SubscribeBooks(Level3{})
		So(err, ShouldNotBeNil)
	})

	Convey("SubscribeBooks dials, subscribes, and registers a batch socket", t, func() {
		var (
			subscribeFrames int32
			readerStarted   int32
		)

		upgrader := gorilla.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}

			go func() {
				for {
					_, payload, err := conn.ReadMessage()
					if err != nil {
						return
					}

					if strings.Contains(string(payload), "subscribe") {
						atomic.AddInt32(&subscribeFrames, 1)
					}
				}
			}()
		}))
		defer server.Close()

		auth := kraken.NewAuth()
		auth.SeedToken("l3-subscribe-token")

		instrument := &Instrument{
			System:    runtime.NewSystem(context.Background(), "instrument"),
			auth:      auth,
			Level3:    &sync.Map{},
			symbols:   []string{"BK/USD", "BK2/USD"},
			quote:     "USD",
			level3URL: "ws" + strings.TrimPrefix(server.URL, "http"),
		}

		err := instrument.SubscribeBooks(Level3{
			Reader: func(client *network.WebsocketClient) {
				atomic.AddInt32(&readerStarted, 1)
				go func() {
					for {
						if _, err := client.Read(); err != nil {
							return
						}
					}
				}()
			},
			Stale: func([]string) {},
		})

		So(err, ShouldBeNil)
		So(instrument.Status(), ShouldEqual, runtime.WAITING)
		So(atomic.LoadInt32(&readerStarted), ShouldEqual, 1)
		So(atomic.LoadInt32(&subscribeFrames), ShouldBeGreaterThan, 0)

		stored := 0
		instrument.Level3.Range(func(_, _ any) bool {
			stored++
			return true
		})
		So(stored, ShouldEqual, 1)
	})
}
