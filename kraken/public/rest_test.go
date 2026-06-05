package public

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	Convey("Given a Kraken REST envelope", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {"XXBTZEUR": {"c": ["65000.0", "1.0"]}}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		rest := NewRest(ctx, EndpointType(server.URL))
		defer rest.Close()

		Convey("It should decode result into the model", func() {
			var response map[string]any
			err := rest.Get(ctx, fiber.Map{"pair": "BTC/EUR"}, &response)

			So(err, ShouldBeNil)
			So(response["XXBTZEUR"], ShouldNotBeNil)
		})
	})
}

func TestRestGetKrakenError(t *testing.T) {
	Convey("Given a Kraken REST error envelope", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": ["EGeneral:Invalid arguments"],
				"result": null
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		rest := NewRest(ctx, EndpointType(server.URL))
		defer rest.Close()

		Convey("It should return the exchange error", func() {
			var response map[string]any
			err := rest.Get(ctx, fiber.Map{"pair": "BTC/EUR"}, &response)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "EGeneral:Invalid arguments")
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
