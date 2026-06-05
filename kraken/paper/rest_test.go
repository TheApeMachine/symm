package paper

import (
	"context"
	"testing"

	"github.com/gofiber/fiber/v3"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRestGetPost(t *testing.T) {
	Convey("Given a paper REST client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rest, err := NewRest(ctx)

		So(err, ShouldBeNil)

		Convey("It should reject GET requests", func() {
			var payload any

			So(rest.Get(ctx, fiber.Map{}, &payload), ShouldNotBeNil)
		})

		Convey("It should return a websocket token on POST", func() {
			var result struct {
				Token   string `json:"token"`
				Expires int    `json:"expires"`
			}

			So(rest.Post(ctx, fiber.Map{}, &result), ShouldBeNil)
			So(result.Token, ShouldNotBeEmpty)
			So(result.Expires, ShouldBeGreaterThan, 0)
			So(rest.Close(), ShouldBeNil)
			So(rest.Error(), ShouldBeNil)
		})
	})
}
