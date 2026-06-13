package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func testPairCatalog(t *testing.T) *PairCatalog {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, writeErr := writer.Write([]byte(`{
			"error": [],
			"result": {
				"XXBTZUSD": {
					"altname": "XBTUSD",
					"wsname": "BTC/USD",
					"quote": "ZUSD",
					"fee_volume_currency": "ZUSD",
					"fees": [[0, 0.26], [50000, 0.24]],
					"fees_maker": [[0, 0.16]],
					"tick_size": "0.1"
				}
			}
		}`))

		if writeErr != nil {
			t.Error(writeErr)
		}
	}))
	t.Cleanup(server.Close)

	catalog := NewPairCatalog(context.Background())
	catalog.assetPairsAPI = public.EndpointType(server.URL)

	return catalog
}

func TestPairCatalogFeeRate(t *testing.T) {
	Convey("Given a catalog backed by AssetPairs REST", t, func() {
		catalog := testPairCatalog(t)

		Convey("It should return published taker and maker rates", func() {
			taker, err := catalog.FeeRate("BTC/USD", trading.Market)

			So(err, ShouldBeNil)
			So(taker, ShouldAlmostEqual, 0.0026, 1e-12)

			maker, err := catalog.FeeRate("BTC/USD", trading.Limit)

			So(err, ShouldBeNil)
			So(maker, ShouldAlmostEqual, 0.0016, 1e-12)
		})

		Convey("It should advance fee tiers as volume accumulates", func() {
			So(catalog.RecordFill("BTC/USD", 50_000), ShouldBeNil)

			taker, err := catalog.FeeRate("BTC/USD", trading.Market)

			So(err, ShouldBeNil)
			So(taker, ShouldAlmostEqual, 0.0024, 1e-12)
		})

		Convey("It should reject unknown symbols", func() {
			_, err := catalog.FeeRate("ETH/USD", trading.Market)

			So(err, ShouldNotBeNil)
		})

		Convey("It should resolve REST pair names", func() {
			restPair, err := catalog.RestPair("BTC/USD")

			So(err, ShouldBeNil)
			So(restPair, ShouldEqual, "XBTUSD")
		})
	})
}

func TestPairCatalogCachesRESTMetadata(t *testing.T) {
	Convey("Given repeated metadata lookups for the same paper catalog", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"altname": "XBTUSD",
						"wsname": "BTC/USD",
						"quote": "ZUSD",
						"fee_volume_currency": "ZUSD",
						"fees": [[0, 0.26], [50000, 0.24]],
						"fees_maker": [[0, 0.16]],
						"tick_size": "0.1"
					}
				}
			}`))
		}))
		defer server.Close()

		catalog := NewPairCatalog(context.Background())
		catalog.assetPairsAPI = public.EndpointType(server.URL)

		_, feeErr := catalog.FeeRate("BTC/USD", trading.Market)
		_, restErr := catalog.RestPair("BTC/USD")
		fillErr := catalog.RecordFill("BTC/USD", 50_000)
		_, tickErr := catalog.TickSize("BTC/USD")

		Convey("It should fetch AssetPairs once and reuse the session cache", func() {
			So(feeErr, ShouldBeNil)
			So(restErr, ShouldBeNil)
			So(fillErr, ShouldBeNil)
			So(tickErr, ShouldBeNil)
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestPairCatalogCoalescesConcurrentAssetPairs(t *testing.T) {
	Convey("Given concurrent metadata lookups during a burst", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"altname": "XBTUSD",
						"wsname": "BTC/USD",
						"quote": "ZUSD",
						"fee_volume_currency": "ZUSD",
						"fees": [[0, 0.26]],
						"fees_maker": [[0, 0.16]],
						"tick_size": "0.1"
					},
					"XETHZUSD": {
						"altname": "ETHUSD",
						"wsname": "ETH/USD",
						"quote": "ZUSD",
						"fee_volume_currency": "ZUSD",
						"fees": [[0, 0.26]],
						"fees_maker": [[0, 0.16]],
						"tick_size": "0.01"
					}
				}
			}`))
		}))
		defer server.Close()

		catalog := NewPairCatalog(context.Background())
		catalog.assetPairsAPI = public.EndpointType(server.URL)

		const workers = 16
		start := make(chan struct{})
		errors := make(chan error, workers)

		for worker := 0; worker < workers; worker++ {
			go func(index int) {
				<-start
				symbol := "BTC/USD"

				if index%2 == 1 {
					symbol = "ETH/USD"
				}

				_, err := catalog.FeeRate(symbol, trading.Market)
				errors <- err
			}(worker)
		}

		close(start)

		for worker := 0; worker < workers; worker++ {
			So(<-errors, ShouldBeNil)
		}

		Convey("It should coalesce the AssetPairs fetch", func() {
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestPairCatalogCoalescesConcurrentDepth(t *testing.T) {
	testconfig.Load(t)

	Convey("Given concurrent depth lookups during a burst", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"asks": [["50000.0", "1.0", 1781285552]],
						"bids": [["49900.0", "1.0", 1781285552]]
					}
				}
			}`))
		}))
		defer server.Close()

		catalog := NewPairCatalog(context.Background())
		catalog.depthAPI = public.EndpointType(server.URL)

		const workers = 12
		start := make(chan struct{})
		errors := make(chan error, workers)

		for worker := 0; worker < workers; worker++ {
			go func() {
				<-start
				_, err := catalog.DepthBook("XBTUSD", 10)
				errors <- err
			}()
		}

		close(start)

		for worker := 0; worker < workers; worker++ {
			So(<-errors, ShouldBeNil)
		}

		Convey("It should coalesce the depth fetch", func() {
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestPairCatalogCachesDepthWithinQuoteAge(t *testing.T) {
	testconfig.Load(t)

	Convey("Given repeated depth lookups inside max quote age", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"asks": [["50000.0", "1.0", 1781285552]],
						"bids": [["49900.0", "1.0", 1781285552]]
					}
				}
			}`))
		}))
		defer server.Close()

		catalog := NewPairCatalog(context.Background())
		catalog.depthAPI = public.EndpointType(server.URL)

		first, firstErr := catalog.DepthBook("XBTUSD", 10)
		second, secondErr := catalog.DepthBook("XBTUSD", 10)

		Convey("It should reuse the cached depth snapshot", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(first.Asks[0][0], ShouldEqual, "50000.0")
			So(second.Asks[0][0], ShouldEqual, "50000.0")
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestDepthVWAP(t *testing.T) {
	Convey("Given ask-side depth rows", t, func() {
		levels := [][]any{
			{"100", "1"},
			{"102", "1"},
		}

		price, err := depthVWAP(levels, 1.5)

		So(err, ShouldBeNil)
		So(price, ShouldAlmostEqual, 100.6666666667, 1e-6)
	})
}

func BenchmarkPairCatalogFeeRate(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{
			"error": [],
			"result": {
				"XXBTZUSD": {
					"wsname": "BTC/USD",
					"fees": [[0, 0.26]],
					"fees_maker": [[0, 0.16]]
				}
			}
		}`))
	}))
	b.Cleanup(server.Close)

	catalog := NewPairCatalog(context.Background())
	catalog.assetPairsAPI = public.EndpointType(server.URL)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = catalog.FeeRate("BTC/USD", trading.Market)
	}
}

func BenchmarkPairCatalogDepthBook(b *testing.B) {
	testconfig.MustLoad()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{
			"error": [],
			"result": {
				"XXBTZUSD": {
					"asks": [["50000.0", "1.0", 1781285552]],
					"bids": [["49900.0", "1.0", 1781285552]]
				}
			}
		}`))
	}))
	b.Cleanup(server.Close)

	catalog := NewPairCatalog(context.Background())
	catalog.depthAPI = public.EndpointType(server.URL)

	b.ReportAllocs()

	for b.Loop() {
		_, err := catalog.DepthBook("XBTUSD", 10)

		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDepthVWAP(b *testing.B) {
	levels := [][]any{
		{"100", "1"},
		{"101", "1"},
		{"102", "1"},
	}
	b.ResetTimer()

	for b.Loop() {
		_, err := depthVWAP(levels, 2.5)

		if err != nil {
			b.Fatal(err)
		}
	}
}
