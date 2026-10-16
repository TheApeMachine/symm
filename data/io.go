package data

import (
	"encoding/binary"
	"io"

	"github.com/theapemachine/errnie"
)

/*
Size returns the exact total byte count of the serialized Value.
*/
func (value *Value) Size() int {
	layers, index := value.filled()
	total := headerSize + 1 + len(value.Origin) + 1 + len(value.Role) + 1 + len(value.Scope) + 2
	for i := range layers {
		total += 2 + len(index[i]) + 4 + len(layers[i])
	}
	return total
}

/*
headerSize is the fixed numerical header: ID, Tick, SeqIdx, Timestamp, Epoch.
*/
const headerSize = 48

/*
Read serializes Value into p in one pass.
Returns (n, io.EOF) on success, or io.ErrShortBuffer if len(p) < Size().
Implements io.Reader.
*/
func (value *Value) Read(p []byte) (n int, err error) {
	needed := value.Size()

	if len(p) < needed {
		return 0, io.ErrShortBuffer
	}

	layers, index := value.filled()

	// 1. Fixed numerical header (headerSize bytes)
	copy(p[0:16], value.ID[:])
	binary.LittleEndian.PutUint64(p[16:24], value.Tick)
	binary.LittleEndian.PutUint64(p[24:32], value.SeqIdx)
	binary.LittleEndian.PutUint64(p[32:40], value.Timestamp)
	binary.LittleEndian.PutUint64(p[40:48], value.Epoch)

	offset := headerSize

	// 2. Identity strings (Origin, Role, Scope)
	p[offset] = byte(len(value.Origin))
	offset++
	copy(p[offset:], value.Origin)
	offset += len(value.Origin)

	p[offset] = byte(len(value.Role))
	offset++
	copy(p[offset:], value.Role)
	offset += len(value.Role)

	p[offset] = byte(len(value.Scope))
	offset++
	copy(p[offset:], value.Scope)
	offset += len(value.Scope)

	// 3. Layer count (uint16)
	binary.LittleEndian.PutUint16(p[offset:offset+2], uint16(len(layers)))
	offset += 2

	// 4. Layers
	for i := range layers {
		idx := index[i]
		binary.LittleEndian.PutUint16(p[offset:offset+2], uint16(len(idx)))
		offset += 2
		copy(p[offset:offset+len(idx)], idx)
		offset += len(idx)

		layer := layers[i]
		lLen := len(layer)
		binary.LittleEndian.PutUint32(p[offset:offset+4], uint32(lLen))
		offset += 4
		copy(p[offset:offset+lLen], layer)
		offset += lLen
	}

	return offset, io.EOF
}

/*
Write unserializes a Value from p.
Implements io.Writer.
*/
func (value *Value) Write(p []byte) (n int, err error) {
	if len(p) < headerSize+5 {
		return 0, io.ErrUnexpectedEOF
	}

	copy(value.ID[:], p[0:16])
	value.Tick = binary.LittleEndian.Uint64(p[16:24])
	value.SeqIdx = binary.LittleEndian.Uint64(p[24:32])
	value.Timestamp = binary.LittleEndian.Uint64(p[32:40])
	value.Epoch = binary.LittleEndian.Uint64(p[40:48])
	value.slots = 0

	offset := headerSize

	// Origin
	oLen := int(p[offset])
	offset++
	if len(p) < offset+oLen {
		return offset, io.ErrUnexpectedEOF
	}
	value.Origin = string(p[offset : offset+oLen])
	offset += oLen

	// Role
	if len(p) < offset+1 {
		return offset, io.ErrUnexpectedEOF
	}
	rLen := int(p[offset])
	offset++
	if len(p) < offset+rLen {
		return offset, io.ErrUnexpectedEOF
	}
	value.Role = string(p[offset : offset+rLen])
	offset += rLen

	// Scope
	if len(p) < offset+1 {
		return offset, io.ErrUnexpectedEOF
	}
	sLen := int(p[offset])
	offset++
	if len(p) < offset+sLen {
		return offset, io.ErrUnexpectedEOF
	}
	value.Scope = string(p[offset : offset+sLen])
	offset += sLen

	value.FormatPrefix()

	// Layer count
	if len(p) < offset+2 {
		return offset, io.ErrUnexpectedEOF
	}
	numLayers := int(binary.LittleEndian.Uint16(p[offset : offset+2]))
	offset += 2

	// Layers
	value.Layers = make([][]byte, numLayers)
	value.Index = make([]string, numLayers)
	for i := range numLayers {
		if len(p) < offset+2 {
			return offset, io.ErrUnexpectedEOF
		}
		idxLen := int(binary.LittleEndian.Uint16(p[offset : offset+2]))
		offset += 2
		if len(p) < offset+idxLen {
			return offset, io.ErrUnexpectedEOF
		}
		value.Index[i] = string(p[offset : offset+idxLen])
		offset += idxLen

		if len(p) < offset+4 {
			return offset, io.ErrUnexpectedEOF
		}
		lLen := int(binary.LittleEndian.Uint32(p[offset : offset+4]))
		offset += 4
		if len(p) < offset+lLen {
			return offset, io.ErrUnexpectedEOF
		}
		value.Layers[i] = p[offset : offset+lLen]
		offset += lLen
	}

	return offset, nil
}

/*
From unserializes a Value directly from a byte slice.
*/
func From(p []byte) *Value {
	val := New()
	_, err := val.Write(p)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.From] error writing value",
			err,
		))
	}

	return val
}

/*
WriteTo writes the whole serialized Value to w in a single Write, so
io.Copy(dst, value) hands dst exactly one complete Value per call.
Implements io.WriterTo.
*/
func (value *Value) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(value.Bytes())
	return int64(n), err
}

/*
ReadFrom reconstructs Value directly from an incoming stream r.
Implements io.ReaderFrom for zero-intermediate streaming via io.Copy.
*/
func (value *Value) ReadFrom(r io.Reader) (int64, error) {
	var fixed [headerSize]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return 0, err
	}

	copy(value.ID[:], fixed[0:16])
	value.Tick = binary.LittleEndian.Uint64(fixed[16:24])
	value.SeqIdx = binary.LittleEndian.Uint64(fixed[24:32])
	value.Timestamp = binary.LittleEndian.Uint64(fixed[32:40])
	value.Epoch = binary.LittleEndian.Uint64(fixed[40:48])
	value.slots = 0

	total := int64(headerSize)

	readStr := func() (string, error) {
		var lenBuf [1]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return "", err
		}
		total++
		sLen := int(lenBuf[0])
		if sLen == 0 {
			return "", nil
		}
		sBuf := make([]byte, sLen)
		if _, err := io.ReadFull(r, sBuf); err != nil {
			return "", err
		}
		total += int64(sLen)
		return string(sBuf), nil
	}

	var err error
	if value.Origin, err = readStr(); err != nil {
		return total, err
	}
	if value.Role, err = readStr(); err != nil {
		return total, err
	}
	if value.Scope, err = readStr(); err != nil {
		return total, err
	}

	value.FormatPrefix()

	var numBuf [2]byte
	if _, err := io.ReadFull(r, numBuf[:]); err != nil {
		return total, err
	}
	total += 2
	numLayers := int(binary.LittleEndian.Uint16(numBuf[:]))

	value.Layers = make([][]byte, numLayers)
	value.Index = make([]string, numLayers)
	var idxLenBuf [2]byte
	var lenBuf [4]byte
	for i := range numLayers {
		if _, err := io.ReadFull(r, idxLenBuf[:]); err != nil {
			return total, err
		}
		total += 2
		idxLen := int(binary.LittleEndian.Uint16(idxLenBuf[:]))
		if idxLen > 0 {
			idxBytes := make([]byte, idxLen)
			if _, err := io.ReadFull(r, idxBytes); err != nil {
				return total, err
			}
			total += int64(idxLen)
			value.Index[i] = string(idxBytes)
		} else {
			value.Index[i] = ""
		}

		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			return total, err
		}
		total += 4

		lLen := int(binary.LittleEndian.Uint32(lenBuf[:]))
		layer := make([]byte, lLen)
		if _, err := io.ReadFull(r, layer); err != nil {
			return total, err
		}
		total += int64(lLen)
		value.Layers[i] = layer
	}

	return total, nil
}

/*
Bytes returns a newly allocated slice containing the serialized Value.
*/
func (value *Value) Bytes() []byte {
	buf := make([]byte, value.Size())

	// Read reports a complete serialization as io.EOF.
	if _, err := value.Read(buf); err != nil && err != io.EOF {
		errnie.Error(errnie.Err(errnie.IO, "data: value serialization failed", err))
		return nil
	}

	return buf
}
