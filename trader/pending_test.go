package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestPendingRetryBackoffGatesAttempts proves failed reconciles skip OpenOrders
until the dedicated backoff window elapses, then clear after success.
*/
func TestPendingRetryBackoffGatesAttempts(t *testing.T) {
	Convey("Given a pending retry clock", t, func() {
		var retry PendingRetry
		now := time.Unix(1_700_000_000, 0)

		So(retry.Allow(now), ShouldBeTrue)

		retry.Schedule(now)
		So(retry.Allow(now), ShouldBeFalse)
		So(retry.Allow(retry.at), ShouldBeTrue)

		retry.Clear()
		So(retry.Allow(now), ShouldBeTrue)
		So(retry.backoff, ShouldEqual, 0)
	})
}
