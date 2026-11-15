package broker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

type historyClient struct {
	trades []byte
	orders []byte
}

func (c *historyClient) Read(...string) ([]byte, error) { return nil, nil }
func (c *historyClient) Write([]byte) error             { return nil }
func (c *historyClient) Post(url string, _ any) ([]byte, error) {
	if strings.Contains(url, "QueryOrders") {
		if len(c.orders) == 0 {
			return []byte(`{"error":[],"result":{}}`), nil
		}

		return c.orders, nil
	}

	return c.trades, nil
}
func (c *historyClient) Watch(string, any, json.Unmarshaler, func()) {}

const historyTwoClosed = `{
	"error": [],
	"result": {
		"trades": {
			"t1": {
				"ordertxid": "o1",
				"pair": "XBTUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "100.00",
				"cost": "100.00",
				"fee": "0.01",
				"vol": "1.0",
				"time": 1700000000.0
			},
			"t2": {
				"ordertxid": "o2",
				"pair": "XBTUSD",
				"type": "sell",
				"ordertype": "market",
				"price": "110.00",
				"cost": "110.00",
				"fee": "0.01",
				"vol": "1.0",
				"time": 1700000100.0
			},
			"t3": {
				"ordertxid": "o3",
				"pair": "XBTUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "90.00",
				"cost": "90.00",
				"fee": "0.01",
				"vol": "1.0",
				"time": 1700000200.0
			},
			"t4": {
				"ordertxid": "o4",
				"pair": "XBTUSD",
				"type": "sell",
				"ordertype": "market",
				"price": "95.00",
				"cost": "95.00",
				"fee": "0.01",
				"vol": "1.0",
				"time": 1700000300.0
			}
		}
	}
}`

const historyPartialOpen = `{
	"error": [],
	"result": {
		"trades": {
			"t1": {
				"ordertxid": "o1",
				"pair": "ZECUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "40.00",
				"cost": "40.00",
				"fee": "0.01",
				"vol": "1.0",
				"time": 1700000000.0
			},
			"t2": {
				"ordertxid": "o2",
				"pair": "ZECUSD",
				"type": "sell",
				"ordertype": "market",
				"price": "41.00",
				"cost": "16.40",
				"fee": "0.01",
				"vol": "0.4",
				"time": 1700000100.0
			}
		}
	}
}`

const historyTwoOpen = `{
	"error": [],
	"result": {
		"trades": {
			"t1": {
				"ordertxid": "o1",
				"pair": "BERAUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "1.00",
				"cost": "10.00",
				"fee": "0.01",
				"vol": "10.0",
				"time": 1700000000.0
			},
			"t2": {
				"ordertxid": "o2",
				"pair": "PEPEUSD",
				"type": "buy",
				"ordertype": "market",
				"price": "0.01",
				"cost": "5.00",
				"fee": "0.01",
				"vol": "500.0",
				"time": 1700000100.0
			}
		}
	}
}`

func withHistory(body string, f func(*Cache)) func() {
	return func() {
		cache := NewCache(
			context.Background(),
			"real",
			&historyClient{trades: []byte(body)},
			nil,
			nil,
			nil,
		)
		f(cache)
	}
}

func withEmptyCache(f func(*Cache)) func() {
	return func() {
		cache := NewCache(context.Background(), "paper", nil, nil, nil, nil)
		f(cache)
	}
}

func TestNewCache(t *testing.T) {
	Convey("Setup", t, withHistory(historyTwoClosed, func(cache *Cache) {
		Convey("When trades history replays two lifecycles", func() {
			closed := cache.Closed()

			Convey("No positions stay open and two closed rows remain", func() {
				So(len(cache.Open()), ShouldEqual, 0)
				So(len(closed), ShouldEqual, 2)
				So(closed[0].Symbol, ShouldEqual, "XBTUSD")
				So(closed[1].Symbol, ShouldEqual, "XBTUSD")
				So(closed[0].ID, ShouldNotEqual, closed[1].ID)
				So(closed[0].ClosedAt, ShouldNotBeNil)
				So(closed[1].ClosedAt, ShouldNotBeNil)
			})
		})
	}))
}

func TestBound(t *testing.T) {
	Convey("Setup", t, withHistory(historyTwoOpen, func(cache *Cache) {
		So(len(cache.Open()), ShouldEqual, 2)

		bera := decimal.NewFromFloat(10)
		book := market.NewBalance(map[string]any{
			"quote": "USD",
			"assets": []*market.Asset{
				market.NewAsset(nil, "BERA", bera, decimal.Zero, bera),
			},
		})

		Convey("When Bound runs against a partial wallet", func() {
			cache.Bound(book)
			open := cache.Open()

			Convey("Only BERAUSD remains with wallet qty", func() {
				So(len(open), ShouldEqual, 1)
				So(open[0].Symbol, ShouldEqual, "BERAUSD")
				So(open[0].Qty.String(), ShouldEqual, "10")
				So(len(cache.Closed()), ShouldEqual, 0)
			})
		})
	}))

	Convey("Setup waiting position", t, withEmptyCache(func(cache *Cache) {
		pos := NewPosition(cache.Context(), testSymbol, "paper", "path", nil, nil, nil)
		So(pos.Enter("R00/M1", dec("1"), nil), ShouldBeNil)
		So(pos.Status(), ShouldEqual, runtime.WAITING)
		cache.open.Store(testSymbol, pos)

		book := market.NewBalance(map[string]any{
			"quote": "USD",
			"assets": []*market.Asset{
				market.NewAsset(nil, "ETH", decimal.NewFromInt(1), decimal.Zero, decimal.NewFromInt(1)),
			},
		})

		Convey("When Bound sees a WAITING enter not yet in the wallet", func() {
			cache.Bound(book)

			Convey("The position stays open until fill or Drop", func() {
				So(cache.Has(testSymbol), ShouldBeTrue)
				So(len(cache.Open()), ShouldEqual, 1)
				So(cache.Open()[0].Status(), ShouldEqual, runtime.WAITING)
			})
		})
	}))
}

func TestAll(t *testing.T) {
	Convey("Setup", t, withEmptyCache(func(cache *Cache) {
		Convey("When open and closed lanes both hold rows", func() {
			openPos := NewPosition(cache.Context(), "BERAUSD", "paper", "", nil, nil, nil)
			closedPos := NewPosition(cache.Context(), "ZECUSD", "paper", "", nil, nil, nil)
			closed := time.Unix(1, 0).UTC()
			closedPos.ClosedAt = &closed

			cache.open.Store("BERAUSD", openPos)
			cache.closed.Store(closedPos.ID, closedPos)

			all := cache.All()

			Convey("All returns both positions", func() {
				So(len(all), ShouldEqual, 2)
			})
		})
	}))
}

func TestOpen(t *testing.T) {
	Convey("Setup", t, withHistory(historyPartialOpen, func(cache *Cache) {
		Convey("When history leaves a partial exit open", func() {
			open := cache.Open()

			Convey("One open row carries 0.6 qty", func() {
				So(len(open), ShouldEqual, 1)
				So(open[0].Symbol, ShouldEqual, "ZECUSD")
				So(open[0].Qty.String(), ShouldEqual, "0.6")
				So(len(cache.Closed()), ShouldEqual, 0)
			})
		})
	}))
}

func TestClosed(t *testing.T) {
	Convey("Setup", t, withHistory(historyTwoClosed, func(cache *Cache) {
		Convey("When filtering closed by symbol", func() {
			closed := cache.Closed("XBTUSD")

			Convey("Both lifecycles match", func() {
				So(len(closed), ShouldEqual, 2)
			})
		})
	}))
}

func TestHas(t *testing.T) {
	Convey("Setup", t, withHistory(historyPartialOpen, func(cache *Cache) {
		Convey("When the symbol is open", func() {
			So(cache.Has("ZECUSD"), ShouldBeTrue)
			So(cache.Has("XBTUSD"), ShouldBeFalse)
		})
	}))
}

func TestAdd(t *testing.T) {
	Convey("Setup", t, withEmptyCache(func(cache *Cache) {
		live := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)

		Convey("When Add stores a new open position", func() {
			dup := cache.Add(testSymbol, live, true)

			Convey("Add reports not duplicate and Has is true", func() {
				So(dup, ShouldBeFalse)
				So(cache.Has(testSymbol), ShouldBeTrue)
				So(cache.Open(testSymbol)[0].ID, ShouldEqual, live.ID)
			})
		})

		Convey("When Add hits an existing open symbol", func() {
			cache.Add(testSymbol, live, true)
			other := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
			dup := cache.Add(testSymbol, other, true)

			Convey("The incumbent is kept", func() {
				So(dup, ShouldBeTrue)
				So(cache.Open(testSymbol)[0].ID, ShouldEqual, live.ID)
			})
		})
	}))
}

func TestSave(t *testing.T) {
	Convey("Setup", t, withEmptyCache(func(cache *Cache) {
		pos := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)

		Convey("When store is nil", func() {
			err := cache.Save(pos)

			Convey("Save is a no-op success", func() {
				So(err, ShouldBeNil)
			})
		})
	}))
}

func TestRemove(t *testing.T) {
	Convey("Setup", t, withEmptyCache(func(cache *Cache) {
		pos := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
		cache.open.Store(testSymbol, pos)

		Convey("When Remove retires an open symbol", func() {
			ok := cache.Remove(testSymbol)

			Convey("Open clears and closed indexes by id", func() {
				So(ok, ShouldBeTrue)
				So(cache.Has(testSymbol), ShouldBeFalse)
				So(len(cache.Open()), ShouldEqual, 0)
				So(len(cache.Closed()), ShouldEqual, 1)
				So(cache.Closed()[0].ID, ShouldEqual, pos.ID)
			})
		})

		Convey("When Remove misses", func() {
			ok := cache.Remove("MISSING/USD")

			Convey("Remove reports false", func() {
				So(ok, ShouldBeFalse)
			})
		})
	}))
}

func TestDrop(t *testing.T) {
	Convey("Setup", t, withEmptyCache(func(cache *Cache) {
		pos := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
		cache.open.Store(testSymbol, pos)

		Convey("When Drop rejects an open symbol", func() {
			cache.Drop(testSymbol)

			Convey("Open lane clears and closed is untouched", func() {
				So(cache.Has(testSymbol), ShouldBeFalse)
				So(len(cache.Open()), ShouldEqual, 0)
				So(len(cache.Closed()), ShouldEqual, 0)
			})
		})
	}))
}

func BenchmarkNewCache(b *testing.B) {
	body := []byte(historyTwoClosed)
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewCache(ctx, "real", &historyClient{trades: body}, nil, nil, nil)
	}
}

func BenchmarkBound(b *testing.B) {
	var cache *Cache
	withHistory(historyTwoOpen, func(c *Cache) { cache = c })()
	bera := decimal.NewFromFloat(10)
	book := market.NewBalance(map[string]any{
		"quote": "USD",
		"assets": []*market.Asset{
			market.NewAsset(nil, "BERA", bera, decimal.Zero, bera),
		},
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		cache.Bound(book)
	}
}

func BenchmarkAll(b *testing.B) {
	var cache *Cache
	withEmptyCache(func(c *Cache) {
		cache = c
		openPos := NewPosition(cache.Context(), "BERAUSD", "paper", "", nil, nil, nil)
		closedPos := NewPosition(cache.Context(), "ZECUSD", "paper", "", nil, nil, nil)
		closed := time.Unix(1, 0).UTC()
		closedPos.ClosedAt = &closed
		cache.open.Store("BERAUSD", openPos)
		cache.closed.Store(closedPos.ID, closedPos)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cache.All()
	}
}

func BenchmarkOpen(b *testing.B) {
	var cache *Cache
	withHistory(historyPartialOpen, func(c *Cache) { cache = c })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cache.Open()
	}
}

func BenchmarkClosed(b *testing.B) {
	var cache *Cache
	withHistory(historyTwoClosed, func(c *Cache) { cache = c })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cache.Closed("XBTUSD")
	}
}

func BenchmarkHas(b *testing.B) {
	var cache *Cache
	withHistory(historyPartialOpen, func(c *Cache) { cache = c })()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cache.Has("ZECUSD")
	}
}

func BenchmarkAdd(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		var cache *Cache
		withEmptyCache(func(c *Cache) { cache = c })()
		live := NewPosition(ctx, testSymbol, "paper", "", nil, nil, nil)
		_ = cache.Add(testSymbol, live, true)
	}
}

func BenchmarkSave(b *testing.B) {
	var cache *Cache
	var pos *Position
	withEmptyCache(func(c *Cache) {
		cache = c
		pos = NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
	})()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cache.Save(pos)
	}
}

func BenchmarkRemove(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		var cache *Cache
		withEmptyCache(func(c *Cache) {
			cache = c
			pos := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
			cache.open.Store(testSymbol, pos)
		})()
		_ = cache.Remove(testSymbol)
	}
}

func BenchmarkDrop(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		var cache *Cache
		withEmptyCache(func(c *Cache) {
			cache = c
			pos := NewPosition(cache.Context(), testSymbol, "paper", "", nil, nil, nil)
			cache.open.Store(testSymbol, pos)
		})()
		cache.Drop(testSymbol)
	}
}
