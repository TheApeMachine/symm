package signal

import (
	"encoding/binary"
	"math"
)

/*
EncodePayload encodes float64 samples as big-endian payload bytes.
*/
func EncodePayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		binary.BigEndian.PutUint64(payload[index*8:], math.Float64bits(sample))
	}

	return payload
}
