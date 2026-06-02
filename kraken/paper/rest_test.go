package paper

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/gofiber/fiber/v3"
)

func TestRestGetPost(t *testing.T) {
	Convey("Given a paper REST client", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		rest, err := NewRest(ctx)

		So(err, ShouldBeNil)

		Convey("It should accept requests without error", func() {
			var payload any

			So(rest.Get(ctx, fiber.Map{}, &payload), ShouldBeNil)
			So(rest.Post(ctx, fiber.Map{}, &payload), ShouldBeNil)
			So(rest.Close(), ShouldBeNil)
			So(rest.Error(), ShouldBeNil)
		})
	})
}
