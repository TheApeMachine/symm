package data

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
The hub streams one Value per websocket message through io.Copy(hub, value),
and the dashboard's parseValues reads one uint32 length header followed by
exactly that many Value frames. The wire contract therefore is: every Write
to a stream writer is prefixed with the payload length. This test pins that
contract end-to-end the way the hub uses it.
*/
func TestStreamWriteIsLengthPrefixed(t *testing.T) {
	value := NewValue("training", "tick", "BTC/USD")
	value.Append([]float64{0.5}, "train", "regions", "BTC", "USD", "R01")

	var stream bytes.Buffer

	hub := writerFunc(func(p []byte) (int, error) {
		var count [4]byte
		binary.LittleEndian.PutUint32(count[:], uint32(len(p)))
		stream.Write(count[:])
		return stream.Write(p)
	})

	n, err := io.Copy(hub, value)
	assert.NoError(t, err)
	assert.EqualValues(t, value.Size(), n)

	written := stream.Bytes()

	declared := binary.LittleEndian.Uint32(written[:4])
	assert.EqualValues(t, value.Size(), declared)
	assert.Len(t, written, 4+value.Size())

	back := New()
	_, err = back.ReadFrom(bytes.NewReader(written[4 : 4+declared]))
	assert.NoError(t, err)
	assert.Equal(t, value.Origin, back.Origin)
	assert.Equal(t, value.Role, back.Role)
	assert.Equal(t, value.Scope, back.Scope)
	assert.Equal(t, len(value.Index), len(back.Index))
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
