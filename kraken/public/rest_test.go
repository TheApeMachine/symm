package public

import (
	"context"
	"testing"

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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rest := NewRest(ctx, EndpointTypeTicker)
		defer rest.Close()

		Convey("It should get a response", func() {
			var response Response
			err := rest.Get(ctx, fiber.Map{"pair": "BTC/USD"}, &response)

			So(err, ShouldBeNil)
			So(response.Result, ShouldNotBeNil)
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
