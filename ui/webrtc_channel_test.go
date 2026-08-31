package ui

import (
	"encoding/binary"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestEncodeFluidChunk proves every SCTP message is self-identifying: it names
its frame, chunk index, and chunk count, so an unordered, non-retransmitting
receiver can reassemble one complete frame and discard incomplete/obsolete ones.
*/
func TestEncodeFluidChunk(t *testing.T) {
	Convey("Given a segmented logical frame", t, func() {
		frameID := uint32(7)
		chunkCount := uint32(3)

		chunks := [][]byte{
			encodeFluidChunk(frameID, 0, chunkCount, []byte("abc")),
			encodeFluidChunk(frameID, 1, chunkCount, []byte("def")),
			encodeFluidChunk(frameID, 2, chunkCount, []byte("ghi")),
		}

		Convey("each chunk carries the frame identity and its position", func() {
			for index, chunk := range chunks {
				So(len(chunk), ShouldBeGreaterThan, fluidChunkHeaderSize)
				So(chunk[:4], ShouldResemble, []byte{'S', 'F', 'D', '1'})
				So(binary.LittleEndian.Uint32(chunk[4:8]), ShouldEqual, frameID)
				So(
					binary.LittleEndian.Uint32(chunk[8:12]),
					ShouldEqual,
					uint32(index),
				)
				So(binary.LittleEndian.Uint32(chunk[12:16]), ShouldEqual, chunkCount)
			}
		})

		Convey("the frames can be reassembled from chunks in any order", func() {
			ordered := []byte{}

			for _, index := range []int{2, 0, 1} {
				ordered = append(ordered, chunks[index][fluidChunkHeaderSize:]...)
			}

			So(string(ordered), ShouldEqual, "ghiabcdef")
		})
	})
}
