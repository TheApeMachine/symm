package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChunkThresholds(t *testing.T) {
	Convey("Setup", t, func() {
		all := map[string]float64{
			"model/paths/a/enter.bin": 0.5,
			"model/paths/b/enter.bin": 0.5,
			"model/paths/c/enter.bin": 0.5,
		}

		Convey("When chunkThresholds splits an oversized listing", func() {
			chunks := chunkThresholds(all, 120)

			seen := 0
			for _, chunk := range chunks {
				seen += len(chunk)
			}

			Convey("Every path appears once across chunks", func() {
				So(len(chunks), ShouldEqual, 3)
				So(seen, ShouldEqual, 3)
			})
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When closed JSON carries epoch, tick, timestamp, and id", func() {
			var entry closedEntry
			ok := entry.read([]byte(`{"epoch":12,"tick":3,"timestamp":99,"position":{"id":"abc","qty":1}}`))

			Convey("Fields and row map are filled", func() {
				So(ok, ShouldBeTrue)
				So(entry.Epoch, ShouldEqual, uint64(12))
				So(entry.Tick, ShouldEqual, uint64(3))
				So(entry.Timestamp, ShouldEqual, uint64(99))
				So(entry.Row["id"], ShouldEqual, "abc")
			})
		})

		Convey("When stamps are missing from closed JSON", func() {
			var bare closedEntry
			ok := bare.read([]byte(`{"position":{"id":"abc"}}`))

			Convey("Stamps stay zero", func() {
				So(ok, ShouldBeTrue)
				So(bare.Epoch, ShouldEqual, uint64(0))
				So(bare.Tick, ShouldEqual, uint64(0))
				So(bare.Timestamp, ShouldEqual, uint64(0))
			})
		})

		Convey("When input is nil, incomplete, or missing id", func() {
			var missing closedEntry

			Convey("read rejects the payload", func() {
				So(missing.read(nil), ShouldBeFalse)
				So(missing.read([]byte(`{"epoch":1}`)), ShouldBeFalse)
				So(missing.read([]byte(`{"position":{"qty":1}}`)), ShouldBeFalse)
			})
		})
	})
}

func TestDecodePathBook(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When raw is nil", func() {
			Convey("decodePathBook returns nil", func() {
				So(decodePathBook(nil), ShouldBeNil)
			})
		})

		Convey("When raw is valid PathBook JSON", func() {
			book := decodePathBook([]byte(`{"action":"enter","resistance":0.53}`))

			Convey("Action and resistance are decoded", func() {
				So(book, ShouldNotBeNil)
				So(book.Action, ShouldEqual, "enter")
				So(book.Resistance, ShouldEqual, 0.53)
			})
		})

		Convey("When action is missing", func() {
			Convey("decodePathBook returns nil", func() {
				So(decodePathBook([]byte(`{"noise":true}`)), ShouldBeNil)
			})
		})
	})
}

func TestPolicyPathKey(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the storage key already ends in action.bin", func() {
			key := policyPathKey("model/paths/R01/enter.bin", "enter")

			Convey("The key is unchanged", func() {
				So(key, ShouldEqual, "model/paths/R01/enter.bin")
			})
		})

		Convey("When a flat path needs an exit suffix", func() {
			key := policyPathKey("model/paths/R01/R02", "exit")

			Convey("The trie key gains exit.bin", func() {
				So(key, ShouldEqual, "model/paths/R01/R02/exit.bin")
			})
		})

		Convey("When the storage key is a radix leaf", func() {
			key := policyPathKey("model/paths/R01/R02.bin", "enter")

			Convey("The key is unchanged", func() {
				So(key, ShouldEqual, "model/paths/R01/R02.bin")
			})
		})
	})
}

func TestClosedHistoryPrefix(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When prefix is paper, real, or virtual closed history", func() {
			Convey("closedHistoryPrefix is true", func() {
				So(closedHistoryPrefix("paper/closed/foo"), ShouldBeTrue)
				So(closedHistoryPrefix("real/closed/foo"), ShouldBeTrue)
				So(closedHistoryPrefix("virtual/closed/foo"), ShouldBeTrue)
			})
		})

		Convey("When prefix is unrelated", func() {
			Convey("closedHistoryPrefix is false", func() {
				So(closedHistoryPrefix("model/paths/foo"), ShouldBeFalse)
			})
		})
	})
}

func BenchmarkChunkThresholds(b *testing.B) {
	all := map[string]float64{
		"model/paths/a/enter.bin": 0.5,
		"model/paths/b/enter.bin": 0.5,
		"model/paths/c/enter.bin": 0.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = chunkThresholds(all, 120)
	}
}

func BenchmarkRead(b *testing.B) {
	raw := []byte(`{"epoch":12,"tick":3,"timestamp":99,"position":{"id":"abc","qty":1}}`)
	var entry closedEntry

	b.ReportAllocs()

	for b.Loop() {
		_ = entry.read(raw)
	}
}

func BenchmarkDecodePathBook(b *testing.B) {
	raw := []byte(`{"action":"enter","resistance":0.53}`)

	b.ReportAllocs()

	for b.Loop() {
		_ = decodePathBook(raw)
	}
}

func BenchmarkPolicyPathKey(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = policyPathKey("model/paths/R01/R02.bin", "enter")
	}
}

func BenchmarkClosedHistoryPrefix(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = closedHistoryPrefix("paper/closed/foo")
	}
}
