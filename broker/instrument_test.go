package broker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	gorilla "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

func instrumentWSServer(tb testing.TB) (wsURL string, cleanup func()) {
	tb.Helper()
	upgrader := gorilla.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))
	return "ws" + strings.TrimPrefix(server.URL, "http"), server.Close
}

type scriptClient struct {
	mu     sync.Mutex
	reads  [][]byte
	errs   []error
	writes [][]byte
	ri     int
}

func (c *scriptClient) Read(...string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ri >= len(c.reads) && c.ri >= len(c.errs) {
		return nil, errors.New("scriptClient exhausted")
	}

	var buf []byte
	var err error

	if c.ri < len(c.reads) {
		buf = c.reads[c.ri]
	}

	if c.ri < len(c.errs) {
		err = c.errs[c.ri]
	}

	c.ri++

	if err != nil {
		return nil, err
	}

	return buf, nil
}

func (c *scriptClient) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, append([]byte(nil), msg...))
	return nil
}

func (c *scriptClient) Post(string, any) ([]byte, error) { return nil, nil }
func (c *scriptClient) Watch(string, any, json.Unmarshaler, func()) {}

func instrumentSnapshotFrame(symbol string) []byte {
	step := decimal.RequireFromString("0.01")
	snapshot := &kraken.Instrument{Channel: "instrument", Type: "snapshot"}
	base, quote, _ := strings.Cut(symbol, "/")
	snapshot.Data.Pairs = []kraken.InstrumentPair{{
		Symbol: symbol, Base: base, Quote: quote, Status: "online",
		TickSize: &step, QtyMin: &step, CostMin: &step, QtyIncrement: &step,
	}}
	raw, _ := sonic.Marshal(snapshot)
	return raw
}

func testInstrument(tb testing.TB, extra ...kraken.InstrumentPair) *Instrument {
	tb.Helper()

	normalizer := spot.NewNormalizer()
	seedPair(normalizer, testSymbol)

	base, quote, _ := strings.Cut(testSymbol, "/")
	qtyMin := decimal.RequireFromString("0.01")
	costMin := decimal.RequireFromString("1")
	step := decimal.RequireFromString("0.01")
	fee := decimal.RequireFromString("0.40")

	instrument := &Instrument{
		cache: &sync.Map{}, takerFees: &sync.Map{},
		quote: quote, symbols: []string{testSymbol}, normalizer: normalizer,
	}
	instrument.cache.Store(testSymbol, kraken.InstrumentPair{
		Symbol: testSymbol, Base: base, Quote: quote, Status: "online",
		QtyPrecision: 8, PricePrecision: 2, CostPrecision: 2,
		QtyMin: &qtyMin, CostMin: &costMin, QtyIncrement: &step, TickSize: &step,
	})
	instrument.takerFees.Store(testSymbol, &fee)

	for _, pair := range extra {
		symbol := normalizer.Name(pair.Symbol)
		if pair.Quote == quote && pair.Status == "online" {
			instrument.symbols = append(instrument.symbols, symbol)
			instrument.cache.Store(symbol, pair)
		}
	}

	return instrument
}

func TestInstrumentCache(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When Cache injects online quote pairs", func() {
			ethMin := decimal.RequireFromString("0.01")
			instrument.Cache([]kraken.InstrumentPair{
				{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
				{Symbol: "ETH/USD", Base: "ETH", Quote: "EUR", Status: "online", QtyMin: &ethMin},
				{Symbol: "OFF/USD", Base: "OFF", Quote: "USD", Status: "cancel_only", QtyMin: &ethMin},
			})

			Convey("Only online USD pairs land in the map", func() {
				So(instrument.Has("ETH/USD"), ShouldBeTrue)
				So(instrument.Has("OFF/USD"), ShouldBeFalse)
				So(len(instrument.Pairs()), ShouldEqual, 2)
			})
		})
	})
}

func TestInstrumentHas(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the symbol is in the cache", func() {
			So(instrument.Has(testSymbol), ShouldBeTrue)
		})

		Convey("When the symbol is missing", func() {
			So(instrument.Has("MISSING/USD"), ShouldBeFalse)
		})
	})
}

func TestInstrumentSpotOpen(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the pair is online", func() {
			So(instrument.SpotOpen(testSymbol), ShouldBeTrue)
		})

		Convey("When the symbol is empty", func() {
			So(instrument.SpotOpen(""), ShouldBeFalse)
		})

		Convey("When the pair is not cached", func() {
			So(instrument.SpotOpen("MISSING/USD"), ShouldBeFalse)
		})
	})
}

func TestInstrumentPair(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the symbol exists", func() {
			pair := instrument.Pair(testSymbol)

			Convey("QtyMin and status match the snapshot", func() {
				So(pair.Symbol, ShouldEqual, testSymbol)
				So(pair.Status, ShouldEqual, "online")
				So(pair.QtyMin.String(), ShouldEqual, "0.01")
			})
		})

		Convey("When the symbol is missing", func() {
			pair := instrument.Pair("MISSING/USD")

			Convey("Pair is empty", func() {
				So(pair.Symbol, ShouldEqual, "")
			})
		})
	})
}

func TestInstrumentSymbols(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When Symbols is called", func() {
			syms := instrument.Symbols()
			syms[0] = "mutated"

			Convey("A clone is returned", func() {
				So(instrument.Symbols()[0], ShouldEqual, testSymbol)
			})
		})
	})
}

func TestInstrumentTakerFee(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)

		Convey("When the fee table has the symbol", func() {
			fee := instrument.TakerFee(testSymbol)

			Convey("Taker fee is 0.40%", func() {
				So(fee, ShouldNotBeNil)
				So(fee.String(), ShouldEqual, "0.4")
			})
		})

		Convey("When the symbol is unknown", func() {
			So(instrument.TakerFee("MISSING/USD"), ShouldBeNil)
		})
	})
}

func TestInstrumentPairs(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		ethMin := decimal.RequireFromString("0.01")
		instrument.Cache([]kraken.InstrumentPair{
			{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
		})

		Convey("When Pairs walks the cache", func() {
			pairs := instrument.Pairs()

			Convey("Both BTC and ETH are present", func() {
				So(len(pairs), ShouldEqual, 2)
			})
		})
	})
}

func TestNewInstrument(t *testing.T) {
	Convey("Setup", t, func() {
		frame := instrumentSnapshotFrame(testSymbol)
		public := &scriptClient{
			reads: [][]byte{
				[]byte(`{"channel":"heartbeat"}`),
				[]byte(`{"event":"pong"}`),
				frame,
			},
		}

		Convey("When NewInstrument seeds from a snapshot", func() {
			instrument := NewInstrument(context.Background(), public, nil)

			Convey("It reaches READY with the pair cached", func() {
				So(instrument, ShouldNotBeNil)
				So(instrument.Status(), ShouldEqual, runtime.READY)
				So(instrument.Has(testSymbol), ShouldBeTrue)
				So(len(public.writes), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When Write fails before seed", func() {
			fail := &writeFailClient{}
			instrument := NewInstrument(context.Background(), fail, spot.NewNormalizer())

			Convey("Instrument stays BUSY", func() {
				So(instrument, ShouldNotBeNil)
				So(instrument.Status() == runtime.READY, ShouldBeFalse)
			})
		})

		Convey("When a soft read error precedes the snapshot", func() {
			public := &scriptClient{
				reads: [][]byte{nil, frame},
				errs:  []error{errors.New("temporary disconnect"), nil},
			}
			instrument := NewInstrument(context.Background(), public, nil)

			Convey("Resubscribe recovers and READY returns", func() {
				So(instrument.Status(), ShouldEqual, runtime.READY)
				So(instrument.Has(testSymbol), ShouldBeTrue)
				So(len(public.writes), ShouldBeGreaterThan, 1)
			})
		})
	})
}

type writeFailClient struct{}

func (writeFailClient) Read(...string) ([]byte, error) { return nil, errors.New("no read") }
func (writeFailClient) Write([]byte) error             { return errors.New("write failed") }
func (writeFailClient) Post(string, any) ([]byte, error) {
	return nil, errors.New("post failed")
}
func (writeFailClient) Watch(string, any, json.Unmarshaler, func()) {}

func TestSubscribeTrades(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		public := &scriptClient{}
		instrument.public = public
		instrument.System = runtime.NewSystem(context.Background(), "instrument", instrument)
		instrument.Transition(runtime.WAITING)

		Convey("When SubscribeTrades writes batched frames", func() {
			err := instrument.SubscribeTrades()

			Convey("Public socket gets trade subscribe and READY returns", func() {
				So(err, ShouldBeNil)
				So(len(public.writes), ShouldEqual, 1)
				So(instrument.Status(), ShouldEqual, runtime.READY)
			})
		})
	})
}

func TestUnsubscribe(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		public := &scriptClient{}
		instrument.public = public
		instrument.System = runtime.NewSystem(context.Background(), "instrument", instrument)
		instrument.Transition(runtime.READY)

		Convey("When Unsubscribe withdraws trade streams", func() {
			err := instrument.Unsubscribe()

			Convey("Public socket gets unsubscribe and WAITING returns", func() {
				So(err, ShouldBeNil)
				So(len(public.writes), ShouldEqual, 1)
				So(instrument.Status(), ShouldEqual, runtime.WAITING)
			})
		})
	})
}

func TestSubscribeBooks(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		instrument.System = runtime.NewSystem(context.Background(), "instrument", instrument)
		instrument.Level3 = &sync.Map{}
		instrument.auth = kraken.NewAuth()
		instrument.auth.SeedToken("test-token")

		Convey("When SubscribeBooks cannot dial level3", func() {
			instrument.level3URL = "ws://127.0.0.1:1"
			err := instrument.SubscribeBooks(func(*network.WebsocketClient) {})

			Convey("It returns an open error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When the level3 reader is nil", func() {
			err := instrument.SubscribeBooks(nil)

			Convey("Validation fails before dial", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When a local level3 socket accepts the dial", func() {
			wsURL, cleanup := instrumentWSServer(t)
			defer cleanup()
			instrument.level3URL = wsURL
			prevPace := system.Cfg.Market.Subscribe.Pace
			system.Cfg.Market.Subscribe.Pace = 0
			defer func() { system.Cfg.Market.Subscribe.Pace = prevPace }()

			var saw *network.WebsocketClient
			err := instrument.SubscribeBooks(func(client *network.WebsocketClient) {
				saw = client
			})

			Convey("Books subscribe and WAITING returns", func() {
				So(err, ShouldBeNil)
				So(saw, ShouldNotBeNil)
				So(instrument.Status(), ShouldEqual, runtime.WAITING)
				_, ok := instrument.Level3.Load(testSymbol)
				So(ok, ShouldBeTrue)
			})
		})
	})
}

func TestRecoverBooks(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		instrument.System = runtime.NewSystem(context.Background(), "instrument", instrument)
		instrument.Level3 = &sync.Map{}
		instrument.Level3.Store(testSymbol, (*network.WebsocketClient)(nil))
		instrument.auth = kraken.NewAuth()
		instrument.auth.SeedToken("test-token")
		instrument.level3URL = "ws://127.0.0.1:1"

		stale := make(chan []string, 1)

		Convey("When RecoverBooks folds a failed batch", func() {
			instrument.RecoverBooks(
				func(*network.WebsocketClient) {},
				[]string{testSymbol, testSymbol, ""},
				func(symbols []string) { stale <- symbols },
			)

			Convey("Stale callback fires and the loop exits", func() {
				select {
				case symbols := <-stale:
					So(symbols, ShouldResemble, []string{testSymbol})
				case <-time.After(2 * time.Second):
					So("stale callback", ShouldEqual, "timed out")
				}

				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					instrument.resubMu.Lock()
					running := instrument.resubRunning
					instrument.resubMu.Unlock()
					if !running {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				instrument.resubMu.Lock()
				So(instrument.resubRunning, ShouldBeFalse)
				instrument.resubMu.Unlock()
			})
		})

		Convey("When a second RecoverBooks stacks while one runs", func() {
			instrument.resubMu.Lock()
			instrument.resubRunning = true
			instrument.resubMu.Unlock()

			instrument.RecoverBooks(func(*network.WebsocketClient) {}, []string{"ETH/USD"}, nil)

			Convey("Pending grows without starting another loop", func() {
				instrument.resubMu.Lock()
				pending := append([]string(nil), instrument.resubPending...)
				instrument.resubRunning = false
				instrument.resubPending = nil
				instrument.resubMu.Unlock()
				So(pending, ShouldResemble, []string{"ETH/USD"})
			})
		})
	})
}

func TestUniqueSymbols(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When duplicates and empties arrive", func() {
			So(uniqueSymbols([]string{"A", "", "A", "B"}), ShouldResemble, []string{"A", "B"})
		})
	})
}

func TestLevel3Victims(t *testing.T) {
	Convey("Setup", t, func() {
		instrument := testInstrument(t)
		instrument.Level3 = &sync.Map{}
		instrument.Level3.Store("BTC/USD|ETH/USD", (*network.WebsocketClient)(nil))
		instrument.Level3.Store("SOL/USD", (*network.WebsocketClient)(nil))

		Convey("When symbols match one batch", func() {
			victims := instrument.level3Victims([]string{"ETH/USD"})

			Convey("Only the overlapping batch is returned", func() {
				So(len(victims), ShouldEqual, 1)
				So(victims[0].key, ShouldEqual, "BTC/USD|ETH/USD")
			})
		})

		Convey("When symbols match nothing", func() {
			victims := instrument.level3Victims([]string{"DOGE/USD"})

			Convey("All batches are victims", func() {
				So(len(victims), ShouldEqual, 2)
			})
		})
	})
}

func BenchmarkInstrumentCache(b *testing.B) {
	instrument := testInstrument(b)
	ethMin := decimal.RequireFromString("0.01")
	pairs := []kraken.InstrumentPair{
		{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		instrument.Cache(pairs)
	}
}

func BenchmarkInstrumentHas(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Has(testSymbol)
	}
}

func BenchmarkInstrumentSpotOpen(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.SpotOpen(testSymbol)
	}
}

func BenchmarkInstrumentPair(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Pair(testSymbol)
	}
}

func BenchmarkInstrumentSymbols(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Symbols()
	}
}

func BenchmarkInstrumentTakerFee(b *testing.B) {
	instrument := testInstrument(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.TakerFee(testSymbol)
	}
}

func BenchmarkInstrumentPairs(b *testing.B) {
	instrument := testInstrument(b)
	ethMin := decimal.RequireFromString("0.01")
	instrument.Cache([]kraken.InstrumentPair{
		{Symbol: "ETH/USD", Base: "ETH", Quote: "USD", Status: "online", QtyMin: &ethMin},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = instrument.Pairs()
	}
}
