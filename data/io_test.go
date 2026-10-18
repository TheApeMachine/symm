package data

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValue_RoundTrip(t *testing.T) {
	Convey("Given a populated Value", t, func() {
		v := NewValue("kraken", "trade", "BTC/USD")
		v.Append([]byte("orderbook_l3_snapshot_data"), "book")
		v.Append([]byte("snm_normalized_movement_layer"), "snm")
		v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")

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
			So(v2.Prefix, ShouldBeEmpty)
			So(len(v2.Layers), ShouldEqual, 3)
			So(v2.Layers[0], ShouldResemble, v.Layers[0])
			So(v2.Index, ShouldResemble, v.Index)
		})

		Convey("a Prefix already set stores the written bytes as a layer", func() {
			payload := []byte("layer-bytes")
			n, err := v.Write(payload)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, len(payload))
			So(v.Origin, ShouldEqual, "kraken")
			So(v.Layers[len(v.Layers)-1], ShouldResemble, payload)
			So(v.Index[len(v.Index)-1], ShouldEqual, v.Prefix)
			payload[0] = 'X'
			So(v.Layers[len(v.Layers)-1][0], ShouldEqual, byte('l'))
		})

		Convey("From helper deserializes a byte slice into Value", func() {
			raw := v.Bytes()
			v3 := From(raw)
			So(v3.ID, ShouldEqual, v.ID)
			So(v3.Layers[1], ShouldResemble, v.Layers[1])
			So(v3.Index, ShouldResemble, v.Index)
			So(v3.Prefix, ShouldBeEmpty)
		})

		Convey("WriteTo hands a seeded prefix to a store that keeps keys", func() {
			v.Prefix = "policy/current/R01/R05/enter.bin"
			sink := &prefixSink{}
			n, err := v.WriteTo(sink)
			So(err, ShouldBeNil)
			So(int(n), ShouldEqual, v.Size())
			So(sink.prefix, ShouldEqual, v.Prefix)
			So(len(sink.body), ShouldEqual, v.Size())
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

type prefixSink struct {
	prefix string
	body   []byte
}

func (sink *prefixSink) Write(p []byte) (int, error) {
	return len(p), nil
}

func (sink *prefixSink) WriteAtPrefix(prefix string, p []byte) (int, error) {
	sink.prefix = prefix
	sink.body = append([]byte(nil), p...)
	return len(p), nil
}

func BenchmarkValue_Bytes(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append([]byte("orderbook_l3_snapshot_data"), "book")
	v.Append([]byte("snm_normalized_movement_layer"), "snm")
	v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")

	b.ReportAllocs()
	for b.Loop() {
		_ = v.Bytes()
	}
}

func BenchmarkValue_Read(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append([]byte("orderbook_l3_snapshot_data"), "book")
	v.Append([]byte("snm_normalized_movement_layer"), "snm")
	v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")

	buf := make([]byte, v.Size())

	b.ReportAllocs()
	for b.Loop() {
		_, _ = v.Read(buf)
	}
}

func BenchmarkValue_Write(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append([]byte("orderbook_l3_snapshot_data"), "book")
	v.Append([]byte("snm_normalized_movement_layer"), "snm")
	v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")
	raw := v.Bytes()

	var v2 Value

	b.ReportAllocs()
	for b.Loop() {
		_, _ = v2.Write(raw)
	}
}

func BenchmarkValue_WriteTo(b *testing.B) {
	v := NewValue("kraken", "trade", "BTC/USD")
	v.Append([]byte("orderbook_l3_snapshot_data"), "book")
	v.Append([]byte("snm_normalized_movement_layer"), "snm")
	v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")

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
	v.Append([]byte("orderbook_l3_snapshot_data"), "book")
	v.Append([]byte("snm_normalized_movement_layer"), "snm")
	v.Append([]byte{0x01, 0x02, 0x03, 0x04}, "raw")
	raw := v.Bytes()

	reader := bytes.NewReader(raw)
	var v2 Value

	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(raw)
		_, _ = v2.ReadFrom(reader)
	}
}
