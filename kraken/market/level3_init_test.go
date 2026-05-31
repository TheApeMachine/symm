package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubLevel3Token struct {
	token string
}

func (stub *stubLevel3Token) Token(context.Context) (string, error) {
	return stub.token, nil
}

func TestConfigureLevel3(t *testing.T) {
	Convey("Given empty credentials", t, func() {
		defer SetLevel3TokenSource(nil)

		err := ConfigureLevel3("", "")

		Convey("It should disable L3 without error", func() {
			So(err, ShouldBeNil)
			So(Level3Available(), ShouldBeFalse)
		})
	})

	Convey("Given a token source", t, func() {
		defer SetLevel3TokenSource(nil)

		SetLevel3TokenSource(&stubLevel3Token{token: "test-token"})

		Convey("Level3Available should report configured", func() {
			So(Level3Available(), ShouldBeTrue)
		})
	})

	Convey("Given replay is active", t, func() {
		defer SetLevel3TokenSource(nil)
		restoreReplay := forceReplayActive(true)
		defer restoreReplay()

		err := ConfigureLevel3("key", "secret")

		Convey("It should disable L3 without dialing Kraken", func() {
			So(err, ShouldBeNil)
			So(Level3Available(), ShouldBeFalse)
		})
	})
}
