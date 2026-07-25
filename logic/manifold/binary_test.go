package manifold

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeLattice(t *testing.T) {
	Convey("Given a 2×2 lattice", t, func() {
		at := time.Unix(1, 0).UTC()
		grid := [][]float64{
			{0, 1},
			{0.5, 0.25},
		}

		Convey("It encodes under the JSON-decimal size for the same grid", func() {
			payload, ok := EncodeLattice(BinaryKindRho, "BTC/USD", at, grid)
			So(ok, ShouldBeTrue)
			key, known := BinaryCacheKey(payload)
			So(known, ShouldBeTrue)
			So(key, ShouldEqual, "manifold_rho")
			So(payload[4], ShouldEqual, BinaryKindRho)
			So(binary.LittleEndian.Uint16(payload[5:7]), ShouldEqual, 2)
			So(binary.LittleEndian.Uint16(payload[7:9]), ShouldEqual, 2)

			minimum := float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[9:13])))
			maximum := float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[13:17])))
			So(minimum, ShouldEqual, 0)
			So(maximum, ShouldEqual, 1)

			// 2×2 u16 payload is 8 bytes; JSON decimals for the same cells are larger.
			So(len(payload), ShouldBeLessThan, 80)
		})
	})
}

func TestEncodeLatticeRejectsEmpty(t *testing.T) {
	Convey("Given an empty grid", t, func() {
		_, ok := EncodeLattice(BinaryKindRho, "BTC/USD", time.Unix(1, 0).UTC(), nil)
		So(ok, ShouldBeFalse)
	})
}

func BenchmarkEncodeLattice(b *testing.B) {
	grid := make([][]float64, 64)

	for row := range grid {
		grid[row] = make([]float64, 64)

		for column := range grid[row] {
			grid[row][column] = float64(row*64+column) / 4096
		}
	}

	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = EncodeLattice(BinaryKindRho, "BTC/USD", at, grid)
	}
}
