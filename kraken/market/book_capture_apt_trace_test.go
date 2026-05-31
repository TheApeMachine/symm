package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func TestCaptureAPTBookSequenceAtRecordedDepth(t *testing.T) {
	capturePath := captureFixturePath(t)

	Convey("Given APT/EUR book frames at recorded depth 25", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		inbound, err := replay.ScanWSRows[BookUpdate](ctx, capturePath, public.BookChannel)

		So(err, ShouldBeNil)

		state := NewBookFeedState("APT/EUR", "apt-trace", 25)

		var frames int
		var firstFailFrame int
		var firstFailChecksum int64

		for update := range inbound {
			if update == nil || update.Symbol != "APT/EUR" {
				continue
			}

			frames++

			wasDiverged := state.Diverged()
			state.Apply(*update)

			if state.Diverged() && !wasDiverged {
				firstFailFrame = frames
				firstFailChecksum = update.Checksum
				t.Logf(
					"APT/EUR diverged after frame=%d kind=%q wire=%d local=%d ready=%v",
					frames,
					update.Kind,
					update.Checksum,
					state.Book().Checksum(),
					state.Ready(),
				)

				break
			}
		}

		Convey("It should stay checksum-aligned through the full APT sequence", func() {
			So(firstFailFrame, ShouldEqual, 0)
		})

		if firstFailFrame > 0 {
			t.Logf(
				"APT/EUR diverged at frame=%d wire_checksum=%d frames_seen=%d",
				firstFailFrame, firstFailChecksum, frames,
			)
		}
	})
}
