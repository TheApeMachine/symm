package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChunkThresholds(t *testing.T) {
	Convey("chunkThresholds splits oversized path listings", t, func() {
		all := map[string]float64{
			"model/paths/a/enter.bin": 0.5,
			"model/paths/b/enter.bin": 0.5,
			"model/paths/c/enter.bin": 0.5,
		}

		chunks := chunkThresholds(all, 120)

		So(len(chunks), ShouldBeGreaterThan, 1)

		seen := 0
		for _, chunk := range chunks {
			seen += len(chunk)
		}
		So(seen, ShouldEqual, 3)
	})
}

func TestClosedEntryRead(t *testing.T) {
	Convey("closedEntry reads the position map and stamps", t, func() {
		var entry closedEntry

		So(entry.read([]byte(`{"epoch":12,"tick":3,"timestamp":99,"position":{"id":"abc","qty":1}}`)), ShouldBeTrue)
		So(entry.Epoch, ShouldEqual, uint64(12))
		So(entry.Tick, ShouldEqual, uint64(3))
		So(entry.Timestamp, ShouldEqual, uint64(99))
		So(entry.Row["id"], ShouldEqual, "abc")

		var bare closedEntry
		So(bare.read([]byte(`{"position":{"id":"abc"}}`)), ShouldBeTrue)
		So(bare.Epoch, ShouldEqual, uint64(0))
		So(bare.Tick, ShouldEqual, uint64(0))
		So(bare.Timestamp, ShouldEqual, uint64(0))

		var missing closedEntry
		So(missing.read(nil), ShouldBeFalse)
		So(missing.read([]byte(`{"epoch":1}`)), ShouldBeFalse)
		So(missing.read([]byte(`{"position":{"qty":1}}`)), ShouldBeFalse)
	})
}

func TestDecodePathBook(t *testing.T) {
	Convey("decodePathBook reads absolute resistance from JSON", t, func() {
		So(decodePathBook(nil), ShouldBeNil)

		book := decodePathBook([]byte(`{"action":"enter","resistance":0.53}`))
		So(book, ShouldNotBeNil)
		So(book.Action, ShouldEqual, "enter")
		So(book.Resistance, ShouldAlmostEqual, 0.53, 1e-9)

		So(decodePathBook([]byte(`{"noise":true}`)), ShouldBeNil)
	})
}

func TestPolicyPathKey(t *testing.T) {
	Convey("policyPathKey keeps .bin keys and appends action", t, func() {
		So(
			policyPathKey("model/paths/R01/enter.bin", "enter"),
			ShouldEqual,
			"model/paths/R01/enter.bin",
		)
		So(
			policyPathKey("model/paths/R01/R02", "exit"),
			ShouldEqual,
			"model/paths/R01/R02/exit.bin",
		)
	})
}
