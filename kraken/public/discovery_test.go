package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

const sampleAssetPairsPayload = `{
	"error": [],
	"result": {
		"XXBTZUSD": {"wsname": "XBT/USD", "status": "online"},
		"XETHZUSD": {"wsname": "ETH/USD", "status": "online"},
		"XETHZEUR": {"wsname": "ETH/EUR", "status": "online"},
		"SOLUSD": {"wsname": "SOL/USD", "status": "online"},
		"OLDUSD": {"wsname": "OLD/USD", "status": "delisted"}
	}
}`

func TestFilterQuoteSymbols(t *testing.T) {
	Convey("Given an AssetPairs payload", t, func() {
		defaults := []string{"BTC/USD"}

		Convey("It should keep online quote matches, cap scan size, and include defaults", func() {
			filtered := FilterQuoteSymbols(
				[]byte(sampleAssetPairsPayload),
				"USD",
				2,
				defaults,
			)

			So(filtered, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		})

		Convey("It should normalize XBT symbols to BTC for websocket v2", func() {
			filtered := FilterQuoteSymbols(
				[]byte(sampleAssetPairsPayload),
				"USD",
				1024,
				nil,
			)

			So(filtered, ShouldContain, "BTC/USD")
			So(filtered, ShouldNotContain, "XBT/USD")
		})

		Convey("It should fall back to defaults when the payload is invalid", func() {
			filtered := FilterQuoteSymbols([]byte("{"), "USD", 1024, defaults)

			So(filtered, ShouldResemble, []string{"BTC/USD"})
		})
	})
}

func TestDiscoverSymbolsRestTree(t *testing.T) {
	Convey("Given a mock AssetPairs REST endpoint", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("market.max_scan_symbols", 1024)
		viper.Set("market.default_symbols", []string{"BTC/USD"})

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(sampleAssetPairsPayload))
		}))

		defer server.Close()

		ctx := context.Background()
		tree := restTestTree(t)
		rest := NewRest(ctx, EndpointTypeAssetPairs, tree)

		defer rest.Close()

		request := assetPairsRequest()

		defer request.Release()

		_ = request.SetMetaValues(map[string]any{
			"method":      "GET",
			"destination": server.URL,
			"headers":     map[string]string{},
		})

		Convey("When the REST artifact request is executed", func() {
			response := rest.Do(ctx, request)

			So(response, ShouldNotBeNil)

			defer response.Release()

			Convey("It should insert a seekable AssetPairs row into the tree", func() {
				found := false

				for inbound := range tree.Seek(AssetPairsTreePrefix()) {
					payload, payloadOK := inbound.PayloadQuiet()
					inbound.Release()

					if payloadOK && string(payload) == sampleAssetPairsPayload {
						found = true
						break
					}
				}

				So(found, ShouldBeTrue)
			})

			Convey("It should discover normalized USD symbols", func() {
				payload, payloadErr := response.Payload()

				So(payloadErr, ShouldBeNil)

				symbols := FilterQuoteSymbols(
					payload,
					"USD",
					1024,
					[]string{"BTC/USD"},
				)

				So(symbols, ShouldContain, "BTC/USD")
				So(symbols, ShouldContain, "ETH/USD")
				So(symbols, ShouldNotContain, "ETH/EUR")
			})
		})
	})
}

func BenchmarkFilterQuoteSymbols(b *testing.B) {
	payload := []byte(sampleAssetPairsPayload)
	defaults := []string{"BTC/USD"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = FilterQuoteSymbols(payload, "USD", 1024, defaults)
	}
}
