package private

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenProviderCachedToken(t *testing.T) {
	Convey("Given a token provider with a warm token", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		provider := &TokenProvider{
			ctx:    ctx,
			cancel: cancel,
			token:  "cached-token",
			until:  time.Now().Add(time.Hour),
		}

		Convey("It should return the cached token without refresh", func() {
			token, err := provider.Token(ctx)

			So(err, ShouldBeNil)
			So(token, ShouldEqual, "cached-token")
		})
	})
}

func TestNewTokenProviderInvalidRest(t *testing.T) {
	Convey("Given empty credentials", t, func() {
		ctx := context.Background()

		_, err := NewTokenProvider(ctx, "", "")

		Convey("It should fail to construct", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
