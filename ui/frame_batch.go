package ui

import "encoding/binary"

const frameBatchHeaderSize = 4

func encodedFrameBatchSize(frames [][]byte) int {
	size := frameBatchHeaderSize + len(frames)*frameBatchHeaderSize

	for _, frame := range frames {
		size += len(frame)
	}

	return size
}

/*
encodeFrameBatch packs ordered JSON frames into one binary websocket payload.
Each frame is prefixed by its byte length; the leading uint32 is the number of
frames. No frame is interpreted or rewritten on the transport path.
*/
func encodeFrameBatch(frames [][]byte) []byte {
	payload := make([]byte, encodedFrameBatchSize(frames))
	binary.LittleEndian.PutUint32(payload, uint32(len(frames)))
	offset := frameBatchHeaderSize

	for _, frame := range frames {
		binary.LittleEndian.PutUint32(payload[offset:], uint32(len(frame)))
		offset += frameBatchHeaderSize
		copy(payload[offset:], frame)
		offset += len(frame)
	}

	return payload
}
