package broker

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

func TestEncodePositionDoc(t *testing.T) {
	Convey("Setup", t, func() {
		data.Epoch.Store(12)
		data.Tick.Store(3)

		pos := NewPosition(t.Context(), "AKE/USD", "paper", "R06.R00/R07.R01", nil, nil, nil)
		opened := time.Unix(1, 0).UTC()
		pos.OpenedAt = opened

		Convey("When a position is encoded for S3", func() {
			body, err := encodePositionDoc(pos, "paper")
			So(err, ShouldBeNil)

			var doc positionStoreDoc
			So(sonic.Unmarshal(body, &doc), ShouldBeNil)

			Convey("Envelope carries stamps and a nested position with book", func() {
				So(doc.Epoch, ShouldEqual, uint64(12))
				So(doc.Tick, ShouldEqual, uint64(3))
				So(doc.Timestamp, ShouldBeGreaterThan, uint64(0))
				So(doc.Position["id"], ShouldEqual, pos.ID)
				So(doc.Position["symbol"], ShouldEqual, "AKE/USD")
				So(doc.Position["book"], ShouldEqual, "paper")
				So(doc.Position["tier_key"], ShouldEqual, "paper")
				So(doc.Position["entry_path"], ShouldEqual, "R06.R00/R07.R01")
			})
		})

		Convey("When position is nil", func() {
			body, err := encodePositionDoc(nil, "paper")

			Convey("Encode fails", func() {
				So(body, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})
	})
}
