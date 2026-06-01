package user

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubToken struct {
	token string
}

func (stub *stubToken) Token(context.Context) (string, error) {
	return stub.token, nil
}

func TestNewBalanceSubscription(t *testing.T) {
	Convey("Given no token source", t, func() {
		ctx := context.Background()

		Convey("It should return an empty feed", func() {
			feed := NewBalanceSubscription(ctx, nil)

			So(feed.Client, ShouldBeNil)
			So(feed.Stream, ShouldBeNil)
		})
	})
}
