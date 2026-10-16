package data

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValue_RoundTrip(t *testing.T) {
	Convey("Given a populated Value", t, func() {
		v := NewValue("kraken", "trade", "BTC/USD")
		v.Append(
			[]byte("orderbook_l3_snapshot_data"),
			[]byte("snm_normalized_movement_layer"),
			[]byte{0x01, 0x02, 0x03, 0x04},
		)

		Convey("serialization to Bytes preserves size and headers", func() {
			raw := v.Bytes()
			So(len(raw), ShouldEqual, v.Size())

			v2 := New()
			nWritten, err := v2.Write(raw)
			So(err, ShouldBeNil)
			So(nWritten, ShouldEqual, len(raw))
			So(v2.ID, ShouldEqual, v.ID)
			So(v2.Tick, ShouldEqual, v.Tick)
			So(v2.SeqIdx, ShouldEqual, v.SeqIdx)
			So(v2.Origin, ShouldEqual, v.Origin)
			So(v2.Role, ShouldEqual, v.Role)
			So(v2.Scope, ShouldEqual, v.Scope)
			So(v2.Prefix, ShouldEqual, v.Prefix)
			So(len(v2.Layers), ShouldEqual, 3)
			So(v2.Layers[0], ShouldResemble, v.Layers[0])
			So(v2.Index, ShouldResemble, v.Index)
		})

		Convey("From helper deserializes a byte slice into Value", func() {
			raw := v.Bytes()
			v3 := From(raw)
			So(v3.ID, ShouldEqual, v.ID)
			So(v3.Layers[1], ShouldResemble, v.Layers[1])
			So(v3.Index, ShouldResemble, v.Index)
			So(v3.Prefix, ShouldEqual, v.Prefix)
		})

		Convey("WriteTo and ReadFrom stream Value through io primitives", func() {
			var buf bytes.Buffer
			n, err := v.WriteTo(&buf)
			So(err, ShouldBeNil)
			So(int(n), ShouldEqual, v.Size())

			v4 := New()
			nRead, err := v4.ReadFrom(&buf)
			So(err, ShouldBeNil)
			So(nRead, ShouldEqual, n)
			So(v4.Layers[1], ShouldResemble, v.Layers[1])
			So(v4.Index, ShouldResemble, v.Index)
		})
	})
}

func BenchmarkValue_Bytes(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append(
		[]byte("orderbook_l3_snapshot_data"),
		[]byte("snm_normalized_movement_layer"),
		[]byte{0x01, 0x02, 0x03, 0x04},
	)

	b.ReportAllocs()
	for b.Loop() {
		_ = v.Bytes()
	}
}

func BenchmarkValue_Read(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append(
		[]byte("orderbook_l3_snapshot_data"),
		[]byte("snm_normalized_movement_layer"),
		[]byte{0x01, 0x02, 0x03, 0x04},
	)

	buf := make([]byte, v.Size())

	b.ReportAllocs()
	for b.Loop() {
		_, _ = v.Read(buf)
	}
}

func BenchmarkValue_Write(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append(
		[]byte("orderbook_l3_snapshot_data"),
		[]byte("snm_normalized_movement_layer"),
		[]byte{0x01, 0x02, 0x03, 0x04},
	)
	raw := v.Bytes()

	var v2 Value

	b.ReportAllocs()
	for b.Loop() {
		_, _ = v2.Write(raw)
	}
}

func BenchmarkValue_WriteTo(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append(
		[]byte("orderbook_l3_snapshot_data"),
		[]byte("snm_normalized_movement_layer"),
		[]byte{0x01, 0x02, 0x03, 0x04},
	)

	var discard bytes.Buffer
	discard.Grow(v.Size())

	b.ReportAllocs()
	for b.Loop() {
		discard.Reset()
		_, _ = v.WriteTo(&discard)
	}
}

func BenchmarkValue_ReadFrom(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append(
		[]byte("orderbook_l3_snapshot_data"),
		[]byte("snm_normalized_movement_layer"),
		[]byte{0x01, 0x02, 0x03, 0x04},
	)
	raw := v.Bytes()

	reader := bytes.NewReader(raw)
	var v2 Value

	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(raw)
		_, _ = v2.ReadFrom(reader)
	}
}
