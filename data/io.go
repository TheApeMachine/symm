package data

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/theapemachine/errnie"
)

/*
Size returns the exact total byte count of the serialized Value.
*/
func (value *Value) Size() int {
	total := 40 + 1 + len(value.Origin) + 1 + len(value.Role) + 1 + len(value.Scope) + 2
	for i := 0; i < len(value.Layers); i++ {
		idxLen := 0
		if i < len(value.Index) {
			idxLen = len(value.Index[i])
		}
		total += 2 + idxLen + 4 + len(value.Layers[i])
	}
	return total
}

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

	// 1. Fixed numerical header (40 bytes)
	copy(p[0:16], value.ID[:])
	binary.LittleEndian.PutUint64(p[16:24], value.Tick)
	binary.LittleEndian.PutUint64(p[24:32], value.SeqIdx)
	binary.LittleEndian.PutUint64(p[32:40], value.Timestamp)

	offset := 40

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
	binary.LittleEndian.PutUint16(p[offset:offset+2], uint16(len(value.Layers)))
	offset += 2

	// 4. Layers
	for i := 0; i < len(value.Layers); i++ {
		idx := ""
		if i < len(value.Index) {
			idx = value.Index[i]
		}
		binary.LittleEndian.PutUint16(p[offset:offset+2], uint16(len(idx)))
		offset += 2
		copy(p[offset:offset+len(idx)], idx)
		offset += len(idx)

		layer := value.Layers[i]
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
	if len(p) < 45 {
		return 0, io.ErrUnexpectedEOF
	}

	copy(value.ID[:], p[0:16])
	value.Tick = binary.LittleEndian.Uint64(p[16:24])
	value.SeqIdx = binary.LittleEndian.Uint64(p[24:32])
	value.Timestamp = binary.LittleEndian.Uint64(p[32:40])

	offset := 40

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

	value.Prefix = fmt.Sprintf(
		"%s/%s/%s/%019d/%019d.bin",
		value.Origin, value.Role, value.Scope, value.Tick, value.SeqIdx,
	)

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
WriteTo writes Value directly to w without allocating an intermediate buffer.
Implements io.WriterTo for zero-copy streaming via io.Copy.
*/
func (value *Value) WriteTo(w io.Writer) (int64, error) {
	var fixed [40]byte
	copy(fixed[0:16], value.ID[:])
	binary.LittleEndian.PutUint64(fixed[16:24], value.Tick)
	binary.LittleEndian.PutUint64(fixed[24:32], value.SeqIdx)
	binary.LittleEndian.PutUint64(fixed[32:40], value.Timestamp)

	n, err := w.Write(fixed[:])
	if err != nil {
		return int64(n), err
	}
	total := int64(n)

	writeStr := func(s string) error {
		var lenBuf [1]byte
		lenBuf[0] = byte(len(s))
		if _, err := w.Write(lenBuf[:]); err != nil {
			return err
		}
		total++
		if len(s) > 0 {
			nw, err := w.Write([]byte(s))
			if err != nil {
				return err
			}
			total += int64(nw)
		}
		return nil
	}

	if err := writeStr(value.Origin); err != nil {
		return total, err
	}
	if err := writeStr(value.Role); err != nil {
		return total, err
	}
	if err := writeStr(value.Scope); err != nil {
		return total, err
	}

	var numBuf [2]byte
	binary.LittleEndian.PutUint16(numBuf[:], uint16(len(value.Layers)))
	if nw, err := w.Write(numBuf[:]); err != nil {
		return total + int64(nw), err
	}
	total += 2

	var idxLenBuf [2]byte
	var lenBuf [4]byte
	for i, layer := range value.Layers {
		idx := ""
		if i < len(value.Index) {
			idx = value.Index[i]
		}
		binary.LittleEndian.PutUint16(idxLenBuf[:], uint16(len(idx)))
		nw, err := w.Write(idxLenBuf[:])
		if err != nil {
			return total + int64(nw), err
		}
		total += int64(nw)

		if len(idx) > 0 {
			nw, err = w.Write([]byte(idx))
			if err != nil {
				return total + int64(nw), err
			}
			total += int64(nw)
		}

		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(layer)))
		nw, err = w.Write(lenBuf[:])
		if err != nil {
			return total + int64(nw), err
		}
		total += int64(nw)

		nw, err = w.Write(layer)
		if err != nil {
			return total + int64(nw), err
		}
		total += int64(nw)
	}

	return total, nil
}

/*
ReadFrom reconstructs Value directly from an incoming stream r.
Implements io.ReaderFrom for zero-intermediate streaming via io.Copy.
*/
func (value *Value) ReadFrom(r io.Reader) (int64, error) {
	var fixed [40]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return 0, err
	}

	copy(value.ID[:], fixed[0:16])
	value.Tick = binary.LittleEndian.Uint64(fixed[16:24])
	value.SeqIdx = binary.LittleEndian.Uint64(fixed[24:32])
	value.Timestamp = binary.LittleEndian.Uint64(fixed[32:40])

	total := int64(40)

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

	value.Prefix = fmt.Sprintf(
		"%s/%s/%s/%019d/%019d.bin",
		value.Origin, value.Role, value.Scope, value.Tick, value.SeqIdx,
	)

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
	_, _ = value.Read(buf)
	return buf
}
