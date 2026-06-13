package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

func TestPairCatalogPrefersLiveBookDuringBurst(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a fresh websocket book snapshot during a fill burst", t, func() {
		catalog := NewPairCatalog(context.Background())
		catalog.ApplyBookUpdate(&market.BookUpdate{
			Symbol: "BTC/USD",
			Bids: []market.BookLevel{
				{Price: 49_900, Qty: 1.5},
				{Price: 49_800, Qty: 2.0},
			},
			Asks: []market.BookLevel{
				{Price: 50_100, Qty: 1.0},
				{Price: 50_200, Qty: 1.5},
			},
			Timestamp: time.Now().UTC(),
		})

		Convey("It should price fills from the live book without REST depth", func() {
			first, firstErr := catalog.DepthForSymbol("BTC/USD", 10)
			second, secondErr := catalog.DepthForSymbol("BTC/USD", 10)

			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(first.Asks[0][0], ShouldEqual, "50100")
			So(second.Asks[0][0], ShouldEqual, "50100")
		})
	})
}

func TestPairCatalogFallsBackToRESTWhenLiveBookStale(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a stale websocket book snapshot", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)

			if request.URL.Query().Get("pair") != "" {
				_, _ = writer.Write([]byte(`{
					"error": [],
					"result": {
						"XXBTZUSD": {
							"asks": [["50000.0", "1.0", 1781285552]],
							"bids": [["49900.0", "1.0", 1781285552]]
						}
					}
				}`))

				return
			}

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
					}
				}
			}`))
		}))
		defer server.Close()

		catalog := NewPairCatalog(context.Background())
		catalog.assetPairsAPI = public.EndpointType(server.URL)
		catalog.depthAPI = public.EndpointType(server.URL)
		catalog.ApplyBookUpdate(&market.BookUpdate{
			Symbol: "BTC/USD",
			Bids:   []market.BookLevel{{Price: 49_900, Qty: 1.0}},
			Asks:   []market.BookLevel{{Price: 50_100, Qty: 1.0}},
			Timestamp: time.Now().UTC().Add(
				-time.Hour,
			),
		})

		book, bookErr := catalog.DepthForSymbol("BTC/USD", 10)

		Convey("It should fetch REST depth once", func() {
			So(bookErr, ShouldBeNil)
			So(book.Asks[0][0], ShouldEqual, "50000.0")
			So(requests.Load(), ShouldEqual, 2)
		})
	})
}

func BenchmarkPairCatalogDepthForSymbolLiveBook(b *testing.B) {
	testconfig.MustLoad()

	catalog := NewPairCatalog(context.Background())
	catalog.ApplyBookUpdate(&market.BookUpdate{
		Symbol:    "BTC/USD",
		Bids:      []market.BookLevel{{Price: 49_900, Qty: 1.5}},
		Asks:      []market.BookLevel{{Price: 50_100, Qty: 1.0}},
		Timestamp: time.Now().UTC(),
	})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = catalog.DepthForSymbol("BTC/USD", 10)
	}
}
