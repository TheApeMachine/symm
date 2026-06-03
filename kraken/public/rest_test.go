package public

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRest(t *testing.T) {
	Convey("Given a parent context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("It should construct a REST client", func() {
			rest := NewRest(ctx, EndpointTypeTicker)
			defer rest.Close()

			So(rest, ShouldNotBeNil)
			So(rest.client, ShouldNotBeNil)
			So(rest.endpoint, ShouldEqual, EndpointTypeTicker)
		})
	})
}

func TestRestGet(t *testing.T) {
	Convey("Given a REST client", t, func() {
		ctx := context.Background()

		rest := NewRest(ctx, EndpointTypeTicker)

		Convey("It should get a response", func() {
			requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			var response map[string]any
			err := rest.Get(requestCtx, fiber.Map{"pair": "BTC/USD"}, &response)

			if err != nil || response["result"] == nil {
				t.Skipf("live Kraken REST unavailable: err=%v result=%v", err, response["result"])
			}

			So(rest.Close(), ShouldBeNil)
		})
	})
}

func TestRestClose(t *testing.T) {
	Convey("Given a REST client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		rest := NewRest(ctx, EndpointTypeTicker)

		Convey("When closed", func() {
			err := rest.Close()
			cancel()

			Convey("It should cancel the context", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkNewRest(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		rest := NewRest(ctx, EndpointTypeTicker)
		_ = rest.Close()
	}
}
