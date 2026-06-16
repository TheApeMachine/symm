package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func seedTradingConfigForCacheTest(test *testing.T) {
	test.Helper()
	seedTradingConfigForCache()
}

func seedTradingConfigForCache() {
	viper.Reset()
	viper.Set("trading.model", "paper")
	viper.Set("trading.max_concurrent_positions", 4)
	viper.Set("trading.entry.opportunity_slot_count", 2)
	viper.Set("trading.max_quote_age", 15*time.Second)
	viper.Set("trading.order_ack_timeout", 500*time.Millisecond)
	viper.Set("trading.entry.transit_ttl", 5*time.Second)
}

func TestRestGetCachesBurstWithinQuoteAge(t *testing.T) {
	seedTradingConfigForCacheTest(t)
	resetGetCache()

	Convey("Given repeated identical GET requests inside max quote age", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {"XXBTZEUR": {"c": ["65000.0", "1.0"]}}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		rest := NewRest(ctx, EndpointType(server.URL))
		defer rest.Close()

		request := fiber.Map{"pair": "BTC/EUR"}

		first := map[string]any{}
		second := map[string]any{}

		Convey("It should fetch once and reuse the cached envelope", func() {
			So(rest.Get(ctx, request, &first), ShouldBeNil)
			So(rest.Get(ctx, request, &second), ShouldBeNil)
			So(first["XXBTZEUR"], ShouldNotBeNil)
			So(second["XXBTZEUR"], ShouldNotBeNil)
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestRestGetCoalescesConcurrentBurst(t *testing.T) {
	seedTradingConfigForCacheTest(t)
	resetGetCache()

	Convey("Given concurrent identical GET requests during a burst", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {"XXBTZEUR": {"c": ["65000.0", "1.0"]}}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		rest := NewRest(ctx, EndpointType(server.URL))
		defer rest.Close()

		const workers = 16
		start := make(chan struct{})
		errors := make(chan error, workers)

		for worker := 0; worker < workers; worker++ {
			go func() {
				<-start
				var response map[string]any
				errors <- rest.Get(ctx, fiber.Map{"pair": "BTC/EUR"}, &response)
			}()
		}

		close(start)

		for worker := 0; worker < workers; worker++ {
			So(<-errors, ShouldBeNil)
		}

		Convey("It should coalesce the upstream fetch", func() {
			So(requests.Load(), ShouldEqual, 1)
		})
	})
}

func TestRestGetDoesNotCacheExchangeErrors(t *testing.T) {
	seedTradingConfigForCacheTest(t)
	resetGetCache()

	Convey("Given a Kraken REST error envelope", t, func() {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			requests.Add(1)
			_, _ = writer.Write([]byte(`{
				"error": ["EGeneral:Invalid arguments"],
				"result": null
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		rest := NewRest(ctx, EndpointType(server.URL))
		defer rest.Close()

		request := fiber.Map{"pair": "BTC/EUR"}

		Convey("It should not cache failed responses", func() {
			var first map[string]any
			So(rest.Get(ctx, request, &first), ShouldNotBeNil)

			var second map[string]any
			So(rest.Get(ctx, request, &second), ShouldNotBeNil)
			So(requests.Load(), ShouldEqual, 2)
		})
	})
}

func BenchmarkRestGetCachedBurst(b *testing.B) {
	seedTradingConfigForCache()
	resetGetCache()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{
			"error": [],
			"result": {"XXBTZEUR": {"c": ["65000.0", "1.0"]}}
		}`))
	}))
	b.Cleanup(server.Close)

	ctx := context.Background()
	rest := NewRest(ctx, EndpointType(server.URL))
	b.Cleanup(func() { _ = rest.Close() })

	request := fiber.Map{"pair": "BTC/EUR"}
	warmup := map[string]any{}

	if err := rest.Get(ctx, request, &warmup); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var response map[string]any

		if err := rest.Get(ctx, request, &response); err != nil {
			b.Fatal(err)
		}
	}
}
