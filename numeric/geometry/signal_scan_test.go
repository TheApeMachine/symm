package geometry

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScanZeroRun(t *testing.T) {
	Convey("Given ScanZeroRun", t, func() {
		Convey("It should return zero start and full length for an all-zero slice", func() {
			words := make([]uint64, 8)
			start, length := ScanZeroRun(words)
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 512)
		})

		Convey("It should return zero start and zero length for an all-ones slice", func() {
			words := []uint64{
				^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0),
				^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0),
			}
			start, length := ScanZeroRun(words)
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 0)
		})

		Convey("It should find the run at bit 64 when the first word is all-ones", func() {
			words := []uint64{^uint64(0), 0, 0, 0, 0, 0, 0, 0}
			start, length := ScanZeroRun(words)
			So(start, ShouldEqual, 64)
			So(length, ShouldEqual, 448)
		})

		Convey("It should find the longest run when multiple runs exist", func() {
			// word 0: 8 zeros then 56 ones — run of 8 at bit 0
			// word 1: 16 zeros then 48 ones — run of 16 at bit 64
			word0 := ^uint64(0xFF)   // bits 0-7 are zero
			word1 := ^uint64(0xFFFF) // bits 0-15 are zero (= bits 64-79 overall)
			words := []uint64{word0, word1, ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)}
			start, length := ScanZeroRun(words)
			So(start, ShouldEqual, 64)
			So(length, ShouldEqual, 16)
		})

		Convey("It should handle a single-word slice", func() {
			words := []uint64{0}
			start, length := ScanZeroRun(words)
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 64)
		})

		Convey("It should handle an empty slice without panic", func() {
			start, length := ScanZeroRun(nil)
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 0)
		})
	})
}

func TestScanOneRun(t *testing.T) {
	Convey("Given ScanOneRun", t, func() {
		Convey("It should return zero start and full length for an all-ones slice", func() {
			words := []uint64{^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0), ^uint64(0)}
			start, length := ScanOneRun(words)
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 512)
		})

		Convey("It should return zero start and zero length for an all-zero slice", func() {
			start, length := ScanOneRun(make([]uint64, 8))
			So(start, ShouldEqual, 0)
			So(length, ShouldEqual, 0)
		})

		Convey("It should find the one-run at the correct offset", func() {
			// first word all-zero, second word all-ones
			words := []uint64{0, ^uint64(0), 0, 0, 0, 0, 0, 0}
			start, length := ScanOneRun(words)
			So(start, ShouldEqual, 64)
			So(length, ShouldEqual, 64)
		})
	})
}

func TestRunLabel(t *testing.T) {
	Convey("Given RunLabel", t, func() {
		Convey("It should be deterministic for the same inputs", func() {
			a := RunLabel(128, 64)
			b := RunLabel(128, 64)
			So(a, ShouldEqual, b)
		})

		Convey("It should produce different labels for different start positions", func() {
			a := RunLabel(0, 64)
			b := RunLabel(128, 64)
			So(a, ShouldNotEqual, b)
		})

		Convey("It should produce different labels for different lengths", func() {
			a := RunLabel(64, 16)
			b := RunLabel(64, 128)
			So(a, ShouldNotEqual, b)
		})

		Convey("It should fit in a uint16 — always less than 65536", func() {
			for start := 0; start < 512; start += 17 {
				for length := 1; length < 512; length += 31 {
					label := RunLabel(start, length)
					So(int(label), ShouldBeLessThan, 65536)
				}
			}
		})
	})
}

func BenchmarkScanZeroRun(b *testing.B) {
	words := make([]uint64, 8)
	words[3] = 0xFFFFFFFFFFFFFFFF

	b.ResetTimer()
	b.ReportAllocs()

	for idx := 0; idx < b.N; idx++ {
		ScanZeroRun(words)
	}
}

func BenchmarkScanOneRun(b *testing.B) {
	words := make([]uint64, 8)
	words[3] = 0xFFFFFFFFFFFFFFFF

	b.ResetTimer()
	b.ReportAllocs()

	for idx := 0; idx < b.N; idx++ {
		ScanOneRun(words)
	}
}

func BenchmarkRunLabel(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for idx := 0; idx < b.N; idx++ {
		RunLabel(idx%512, 64)
	}
}
