package broker

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestLevel3TokenRetry(t *testing.T) {
	floor, ceiling := kraken.TokenRetryFloor, kraken.TokenRetryCeiling
	kraken.TokenRetryFloor, kraken.TokenRetryCeiling = time.Millisecond, 4*time.Millisecond

	defer func() { kraken.TokenRetryFloor, kraken.TokenRetryCeiling = floor, ceiling }()

	Convey("Given a REST endpoint that answers EAPI:Invalid nonce twice, then a token", t, func() {
		instrument := &Instrument{System: runtime.NewSystem(context.Background(), "instrument")}
		calls := 0
		instrument.fetchToken = func() (string, error) {
			calls++

			if calls <= 2 {
				return "", errors.New("EAPI:Invalid nonce")
			}

			return "fresh", nil
		}

		Convey("the reconnect's token fetch retries instead of failing the resubscribe", func() {
			token, err := instrument.level3Token()

			So(err, ShouldBeNil)
			So(token, ShouldEqual, "fresh")
			So(calls, ShouldEqual, 3)

			Convey("and later batches share the fetched token", func() {
				again, err := instrument.level3Token()
				So(err, ShouldBeNil)
				So(again, ShouldEqual, "fresh")
				So(calls, ShouldEqual, 3)
			})
		})

		Convey("a closed instrument stops retrying", func() {
			instrument.fetchToken = func() (string, error) {
				instrument.System.Close()
				return "", errors.New("EAPI:Invalid nonce")
			}

			_, err := instrument.level3Token()
			So(err, ShouldNotBeNil)
		})
	})
}
