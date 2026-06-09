package types

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubTokenRest struct {
	calls int
}

func (stub *stubTokenRest) WebSocketToken(ctx context.Context, token *Token) error {
	stub.calls++

	token.Token = "venue-token"
	token.Expires = 900

	return nil
}

func TestNewToken(t *testing.T) {
	Convey("Given a token rest stub", t, func() {
		ctx := context.Background()
		stub := &stubTokenRest{}

		cachedToken.Store(nil)
		BindTokenRest(stub)

		Convey("It should fetch a new token on first use", func() {
			token, err := NewToken(ctx)

			So(err, ShouldBeNil)
			So(token, ShouldEqual, "venue-token")
			So(stub.calls, ShouldEqual, 1)
		})

		Convey("It should return the cached token while still valid", func() {
			first, err := NewToken(ctx)

			So(err, ShouldBeNil)

			second, err := NewToken(ctx)

			So(err, ShouldBeNil)
			So(second, ShouldEqual, first)
			So(stub.calls, ShouldEqual, 1)
		})
	})
}
