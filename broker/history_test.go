package broker

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

func TestLotRecordJSON(t *testing.T) {
	Convey("A stored lot is epoch, tick, timestamp, and position", t, func() {
		position := &Position{ID: "lot-1", Symbol: "BTC/USD", Book: "paper"}
		record := lotRecord{Epoch: 7, Tick: 11, Timestamp: 19, Position: position}

		raw, err := sonic.Marshal(record)
		So(err, ShouldBeNil)

		restored, err := data.AttributeOk[Position](raw, "position")
		So(err, ShouldBeNil)
		So(restored.ID, ShouldEqual, "lot-1")
		So(restored.Symbol, ShouldEqual, "BTC/USD")
		So(restored.Book, ShouldEqual, "paper")
		So(data.Attribute[uint64](raw, "epoch"), ShouldEqual, 7)
		So(data.Attribute[uint64](raw, "tick"), ShouldEqual, 11)
		So(data.Attribute[uint64](raw, "timestamp"), ShouldEqual, 19)
	})
}
