package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func TestReplayHubDeliversEveryBookFrame(t *testing.T) {
	capturePath := captureFixturePath(t)

	Convey("Given a full book-channel capture", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		scanned, err := countReplayBookFrames(ctx, capturePath)

		So(err, ShouldBeNil)
		So(scanned, ShouldBeGreaterThan, 0)

		hubCtx, hubCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer hubCancel()

		hub, err := replay.Open(capturePath)

		So(err, ShouldBeNil)

		delivered := 0
		inbound := replay.StreamRows[BookUpdate](hubCtx, hub, public.BookChannel)

		for range inbound {
			delivered++
		}

		Convey("Hub playback should deliver every inbound book row", func() {
			So(delivered, ShouldEqual, scanned)
		})
	})
}

func countReplayBookFrames(ctx context.Context, path string) (int, error) {
	inbound, err := replay.ScanWSRows[BookUpdate](ctx, path, public.BookChannel)

	if err != nil {
		return 0, err
	}

	count := 0

	for range inbound {
		count++
	}

	return count, nil
}
