package numeric

import (
	"math/bits"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestXOR(t *testing.T) {
	t.Parallel()

	Convey("XOR toggles overlapping one-bits", t, func() {
		So(XOR(0xf0f0f0f0f0f0f0f0, 0x0f0f0f0f0f0f0f0f), ShouldEqual, ^uint64(0))
	})
}

func TestAND(t *testing.T) {
	t.Parallel()

	Convey("AND keeps only shared one-bits", t, func() {
		So(AND(0xcccccccccccccccc, 0x3333333333333333), ShouldEqual, 0)
	})
}

func TestOR(t *testing.T) {
	t.Parallel()

	Convey("OR merges one-bit patterns", t, func() {
		So(OR(0x00ff00ff00ff00ff, 0xff00ff00ff00ff00), ShouldEqual, ^uint64(0))
	})
}

func TestNOT(t *testing.T) {
	t.Parallel()

	Convey("NOT inverts every bit", t, func() {
		So(NOT(uint64(0)), ShouldEqual, ^uint64(0))
		So(NOT(^uint64(0)), ShouldEqual, uint64(0))
	})
}

func TestSHIFTLEFT(t *testing.T) {
	t.Parallel()

	Convey("SHIFTLEFT shifts bits toward high significance", t, func() {
		So(SHIFTLEFT(1, 4), ShouldEqual, 16)
	})
}

func TestSHIFTRIGHT(t *testing.T) {
	t.Parallel()

	Convey("SHIFTRIGHT shifts bits toward low significance", t, func() {
		So(SHIFTRIGHT(128, 7), ShouldEqual, 1)
	})
}

func TestROTATELEFT(t *testing.T) {
	t.Parallel()

	Convey("ROTATELEFT matches bits.RotateLeft64", t, func() {
		val := uint64(0x8000000000000001)

		So(ROTATELEFT(val, 1), ShouldEqual, bits.RotateLeft64(val, 1))
	})
}

func TestROTATERIGHT(t *testing.T) {
	t.Parallel()

	Convey("ROTATERIGHT matches a left rotation by the negated count", t, func() {
		val := uint64(0x4000000000000003)

		So(ROTATERIGHT(val, 2), ShouldEqual, bits.RotateLeft64(val, -2))
	})
}

func BenchmarkXOR(b *testing.B) {
	var x uint64 = 0x123456789abcdef0

	for b.Loop() {
		x = XOR(x, 0xfedcba9876543210)
	}

	_ = x
}
