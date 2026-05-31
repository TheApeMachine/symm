package market

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/orderbook"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/replay"
)

func TestCaptureAPTFirstSnapshotChecksum(t *testing.T) {
	capturePath := captureFixturePath(t)

	Convey("Given the first APT/EUR book snapshot in the capture", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		inbound, err := replay.ScanWSRows[BookUpdate](ctx, capturePath, public.BookChannel)

		So(err, ShouldBeNil)

		var snapshot *BookUpdate

		for update := range inbound {
			if update == nil || update.Symbol != "APT/EUR" || !update.IsSnapshot() {
				continue
			}

			snapshot = update

			break
		}

		So(snapshot, ShouldNotBeNil)

		book := orderbook.NewBook(orderbook.MaintainDepth(25))
		book.ApplySnapshot(snapshot.BidLevels(), snapshot.AskLevels())

		Convey("It should reproduce the wire checksum from raw level text", func() {
			So(book.Verify(uint32(snapshot.Checksum)), ShouldBeTrue)
		})
	})
}
