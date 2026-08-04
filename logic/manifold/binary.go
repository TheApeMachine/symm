package manifold

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

const (
	binaryHeaderSize  = 26
	binaryKindDisplay = 5
)

/*
EncodeDisplay wraps one backend-composited RGBA8 texture in the SMF1 envelope
consumed by the dashboard. Width and height describe the raw pixel payload;
the symbol and timestamp let the client reject stale or out-of-focus frames.
*/
func EncodeDisplay(
	symbol string,
	at time.Time,
	width, height int,
	rgba []byte,
) ([]byte, error) {
	if symbol == "" || len(symbol) > math.MaxUint8 {
		return nil, fmt.Errorf("manifold display symbol length is invalid")
	}

	if width <= 0 || width > math.MaxUint16 || height <= 0 || height > math.MaxUint16 {
		return nil, fmt.Errorf("manifold display dimensions %dx%d are invalid", width, height)
	}

	if len(rgba) != width*height*4 {
		return nil, fmt.Errorf(
			"manifold display payload has %d bytes for %dx%d RGBA8",
			len(rgba), width, height,
		)
	}

	headerSize := binaryHeaderSize + len(symbol)
	payload := make([]byte, headerSize+len(rgba))
	copy(payload[:4], "SMF1")
	payload[4] = binaryKindDisplay
	binary.LittleEndian.PutUint16(payload[5:7], uint16(width))
	binary.LittleEndian.PutUint16(payload[7:9], uint16(height))
	binary.LittleEndian.PutUint32(payload[9:13], math.Float32bits(0))
	binary.LittleEndian.PutUint32(payload[13:17], math.Float32bits(1))
	binary.LittleEndian.PutUint64(payload[17:25], uint64(at.UnixNano()))
	payload[25] = byte(len(symbol))
	copy(payload[binaryHeaderSize:headerSize], symbol)
	copy(payload[headerSize:], rgba)

	return payload, nil
}
