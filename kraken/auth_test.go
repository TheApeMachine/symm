package kraken

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAuthTokenReuse(t *testing.T) {
	Convey("Given an Auth with a controllable clock and token source", t, func() {
		clock := time.Unix(1_700_000_000, 0)
		fetches := 0
		auth := &Auth{
			now: func() time.Time { return clock },
			fetch: func() (string, error) {
				fetches++
				return "token-" + string(rune('A'+fetches-1)), nil
			},
		}

		Convey("a token within TokenReuse is reused", func() {
			first, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse - time.Second)
			second, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			So(second, ShouldEqual, first)
			So(fetches, ShouldEqual, 1)
		})

		Convey("a token past TokenReuse is reissued for a reconnect", func() {
			first, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			second, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			So(second, ShouldNotEqual, first)
			So(fetches, ShouldEqual, 2)
		})

		Convey("a fetch failure is returned, not masked by the stale token", func() {
			_, err := auth.Token(t.Context())
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			auth.fetch = func() (string, error) { return "", errors.New("rest down") }
			token, err := auth.Token(t.Context())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "rest down")
			So(token, ShouldBeEmpty)
			So(fetches, ShouldEqual, 1) // no retry: the one failed fetch is all

			// The failure caches nothing: the next call fetches again.
			auth.fetch = func() (string, error) { fetches++; return "token-B", nil }
			token, err = auth.Token(t.Context())
			So(err, ShouldBeNil)
			So(token, ShouldEqual, "token-B")
		})

		Convey("an empty token is an error", func() {
			auth.fetch = func() (string, error) { return "", nil }
			_, err := auth.Token(t.Context())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "empty websockets token")
		})

		Convey("a cancelled context fetches nothing", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := auth.Token(ctx)
			So(err, ShouldNotBeNil)
			So(fetches, ShouldEqual, 0)
		})
	})
}
