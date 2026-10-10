package kraken

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRetryToken(t *testing.T) {
	floor, ceiling := TokenRetryFloor, TokenRetryCeiling
	TokenRetryFloor, TokenRetryCeiling = time.Millisecond, 4*time.Millisecond

	defer func() { TokenRetryFloor, TokenRetryCeiling = floor, ceiling }()

	Convey("Given a REST endpoint that answers EAPI:Invalid nonce three times, then a token", t, func() {
		calls := 0
		fetch := func() (string, error) {
			calls++

			if calls <= 3 {
				return "", errors.New("[kraken.auth] failed to retrieve websockets token | EAPI:Invalid nonce")
			}

			return "token", nil
		}

		token, err := RetryToken(context.Background(), "test", fetch)

		So(err, ShouldBeNil)
		So(token, ShouldEqual, "token")
		So(calls, ShouldEqual, 4)
	})

	Convey("Given a permanent credential error, it is returned without retrying", t, func() {
		calls := 0
		_, err := RetryToken(context.Background(), "test", func() (string, error) {
			calls++
			return "", errors.New("EAPI:Invalid key")
		})

		So(err, ShouldNotBeNil)
		So(calls, ShouldEqual, 1)
	})

	Convey("Given a cancelled context, retrying stops", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := RetryToken(ctx, "test", func() (string, error) {
			calls++
			cancel()
			return "", errors.New("EAPI:Invalid nonce")
		})

		So(errors.Is(err, context.Canceled), ShouldBeTrue)
		So(calls, ShouldEqual, 1)
	})
}
