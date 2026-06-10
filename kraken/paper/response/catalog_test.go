package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
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

func TestDepthVWAP(t *testing.T) {
	Convey("Given ask-side depth rows", t, func() {
		levels := [][]string{
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

func BenchmarkDepthVWAP(b *testing.B) {
	levels := [][]string{
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
