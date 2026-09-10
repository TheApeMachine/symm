package tables

import (
	"bytes"
	"math"
	"math/big"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPow10Table(t *testing.T) {
	Convey("Iceberg decimal rescale uses the exact power of ten for each exponent", t, func() {
		for exp := int64(0); exp <= 38; exp++ {
			So(pow10(exp).Cmp(new(big.Int).Exp(big.NewInt(10), big.NewInt(exp), nil)), ShouldEqual, 0)
		}
	})
}

func TestSpan(t *testing.T) {
	Convey("A payload ceiling splits before Arrow's Binary offset limit", t, func() {
		sizes := []int{3000, 3000, 3000}
		end, size, err := span(0, len(sizes), 5000, func(index int) int { return sizes[index] })
		So(err, ShouldBeNil)
		So(end, ShouldEqual, 1)
		So(size, ShouldEqual, 3000)

		end, size, err = span(1, len(sizes), 5000, func(index int) int { return sizes[index] })
		So(err, ShouldBeNil)
		So(end, ShouldEqual, 2)
		So(size, ShouldEqual, 3000)
	})

	Convey("A row larger than the ceiling is still one span", t, func() {
		end, size, err := span(0, 2, 100, func(int) int { return 250 })
		So(err, ShouldBeNil)
		So(end, ShouldEqual, 1)
		So(size, ShouldEqual, 250)
	})

	Convey("A payload that cannot be an Arrow Binary is an error", t, func() {
		_, _, err := span(0, 1, 0, func(int) int { return math.MaxInt32 + 1 })
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "one payload exceeds Arrow Binary")
	})
}

func TestRecords(t *testing.T) {
	Convey("Arrow records preserve variable payload lengths and nulls", t, func() {
		rows := []CaptureRow{
			{Run: "test", Sequence: 1, Payload: []byte("one")},
			{Run: "test", Sequence: 2},
			{Run: "test", Sequence: 3, Payload: bytes.Repeat([]byte("x"), 4096)},
		}
		reader, err := records(CapturesSchema(), len(rows),
			func(index int) int { return len(rows[index].Payload) },
			func(builder *array.RecordBuilder, start, end int) {
				fillCaptures(builder, rows[start:end])
			})
		So(err, ShouldBeNil)
		defer reader.Release()
		index := 0

		for reader.Next() {
			batch := reader.RecordBatch()

			for row := range int(batch.NumRows()) {
				So(num(batch.Column(1), row), ShouldEqual, rows[index].Sequence)
				So(bin(batch.Column(9), row), ShouldResemble, rows[index].Payload)
				index++
			}
		}
		So(reader.Err(), ShouldBeNil)
		So(index, ShouldEqual, len(rows))
	})
}

func BenchmarkRecords(b *testing.B) {
	// A drain of 256 varied precursor payloads models the configured capture
	// drain shape. The bytes are real; no Arrow work is replaced by a mock.
	rows := make([]WitnessRow, 256)

	for index := range rows {
		rows[index] = WitnessRow{Run: "bench", ArtifactKind: "precursor",
			Envelope: EnvelopeRefRow{Run: "bench", Sequence: int64(index + 1)},
			Payload:  bytes.Repeat([]byte("x"), (index%4+1)*4096)}
	}
	schema := WitnessesSchema()
	b.ReportAllocs()

	for b.Loop() {
		reader, err := records(schema, len(rows),
			func(index int) int { return len(rows[index].Payload) },
			func(builder *array.RecordBuilder, start, end int) {
				fillWitnesses(builder, rows[start:end])
			})

		if err != nil {
			b.Fatal(err)
		}
		reader.Release()
	}
}
