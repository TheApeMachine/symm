package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRateLimited(t *testing.T) {
	Convey("RateLimited", t, func() {
		So(RateLimited([]any{"EAPI:Rate limit exceeded"}), ShouldBeTrue)
		So(RateLimited("EAPI:Invalid nonce"), ShouldBeFalse)
		So(VenueErrors([]byte(`{"error":["EAPI:Rate limit exceeded"]}`)), ShouldResemble, []any{"EAPI:Rate limit exceeded"})
	})
}
