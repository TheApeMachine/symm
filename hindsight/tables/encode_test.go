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
		rows := []MeasurementRow{
			{Epoch: 1, Tick: 1, Source: "kraken", Symbol: "BTC/USD", Payload: []byte("one")},
			{Epoch: 1, Tick: 2, Source: "kraken", Symbol: "BTC/USD"},
			{Epoch: 1, Tick: 3, Source: "kraken", Symbol: "BTC/USD", Payload: bytes.Repeat([]byte("x"), 4096)},
		}
		reader, err := records(MeasurementsSchema(), len(rows),
			func(index int) int { return len(rows[index].Payload) },
			func(builder *array.RecordBuilder, start, end int) {
				fillMeasurements(builder, rows[start:end])
			})
		So(err, ShouldBeNil)
		defer reader.Release()
		index := 0

		for reader.Next() {
			batch := reader.RecordBatch()

			for row := range int(batch.NumRows()) {
				So(num(batch.Column(1), row), ShouldEqual, rows[index].Tick)
				So(bin(batch.Column(11), row), ShouldResemble, rows[index].Payload)
				index++
			}
		}
		So(reader.Err(), ShouldBeNil)
		So(index, ShouldEqual, len(rows))
	})
}

func BenchmarkRecords(b *testing.B) {
	rows := make([]MeasurementRow, 256)

	for index := range rows {
		rows[index] = MeasurementRow{
			Epoch:   1,
			Tick:    int64(index + 1),
			Source:  "kraken",
			Symbol:  "BTC/USD",
			Payload: bytes.Repeat([]byte("x"), (index%4+1)*4096),
		}
	}
	schema := MeasurementsSchema()
	b.ReportAllocs()

	for b.Loop() {
		reader, err := records(schema, len(rows),
			func(index int) int { return len(rows[index].Payload) },
			func(builder *array.RecordBuilder, start, end int) {
				fillMeasurements(builder, rows[start:end])
			})

		if err != nil {
			b.Fatal(err)
		}
		reader.Release()
	}
}

