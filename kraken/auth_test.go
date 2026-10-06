package kraken

import (
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
			first, err := auth.Token()
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse - time.Second)
			second, err := auth.Token()
			So(err, ShouldBeNil)
			So(second, ShouldEqual, first)
			So(fetches, ShouldEqual, 1)
		})

		Convey("a token past TokenReuse is reissued for a reconnect", func() {
			first, err := auth.Token()
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			second, err := auth.Token()
			So(err, ShouldBeNil)
			So(second, ShouldNotEqual, first)
			So(fetches, ShouldEqual, 2)
		})

		Convey("a fetch failure is returned, not masked by the stale token", func() {
			_, err := auth.Token()
			So(err, ShouldBeNil)
			clock = clock.Add(TokenReuse)
			auth.fetch = func() (string, error) { return "", errors.New("rest down") }
			token, err := auth.Token()
			So(err, ShouldNotBeNil)
			So(token, ShouldBeEmpty)
		})

		Convey("an empty token is an error", func() {
			auth.fetch = func() (string, error) { return "", nil }
			_, err := auth.Token()
			So(err, ShouldNotBeNil)
		})
	})
}
