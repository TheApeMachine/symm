package ui

import (
	"encoding/binary"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeFrameBatch(t *testing.T) {
	Convey("Given ordered dashboard frames", t, func() {
		frames := [][]byte{
			[]byte(`{"tick":1}`),
			[]byte(`{"tick":2}`),
			[]byte(`{"tick":3}`),
		}
		payload := encodeFrameBatch(frames)

		Convey("It should preserve every frame in order", func() {
			count := binary.LittleEndian.Uint32(payload)
			offset := frameBatchHeaderSize
			decoded := make([][]byte, 0, count)

			for range count {
				length := int(binary.LittleEndian.Uint32(payload[offset:]))
				offset += frameBatchHeaderSize
				decoded = append(decoded, payload[offset:offset+length])
				offset += length
			}

			So(decoded, ShouldResemble, frames)
			So(offset, ShouldEqual, len(payload))
		})
	})
}

func BenchmarkEncodeFrameBatch(b *testing.B) {
	frames := make([][]byte, 32)

	for index := range frames {
		frames[index] = []byte(`{"measurements":{"symbol":"BTC/USD","source":"hawkes","value":0.8125}}`)
	}

	b.ReportAllocs()
	

	for b.Loop() {
		encodeFrameBatch(frames)
	}
}
