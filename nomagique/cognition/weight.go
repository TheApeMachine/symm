package cognition

import (
	"encoding/binary"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
WeightSize is the exact byte length of one packed cognitive weight record.
*/
const WeightSize = 24

/*
PackedWeight is the unpacked reading of one 24-byte cognitive record. It is
plain wire payload: zero methods.
*/
type PackedWeight struct {
	Count     uint64
	Mass      float64
	WriteStep uint64
}

/*
Weight unpacks 24-byte stored records into PackedWeight.
*/
type Weight struct {
	*core.PrimitiveError
	out PackedWeight
}

func NewWeight() *Weight {
	return &Weight{PrimitiveError: core.NewPrimitiveError()}
}

func (weight *Weight) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if weight.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			record := *(*[]byte)(arriving)
			if len(record) < WeightSize {
				weight.Error(fmt.Errorf(
					"%w: cognition: packed weight is %d bytes, want %d",
					core.ErrShape,
					len(record),
					WeightSize,
				))
				return
			}

			weight.out = PackedWeight{
				Count:     binary.LittleEndian.Uint64(record[0:8]),
				Mass:      math.Float64frombits(binary.LittleEndian.Uint64(record[8:16])),
				WriteStep: binary.LittleEndian.Uint64(record[16:24]),
			}

			if !yield(unsafe.Pointer(&weight.out)) {
				return
			}
		}
	}
}

/*
Pack packs a PackedWeight into its 24-byte wire representation.
*/
type Pack struct {
	*core.PrimitiveError
	out [WeightSize]byte
}

func NewPack() *Pack {
	return &Pack{PrimitiveError: core.NewPrimitiveError()}
}

func (pack *Pack) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if pack.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				continue
			}

			pw := (*PackedWeight)(arriving)
			binary.LittleEndian.PutUint64(pack.out[0:8], pw.Count)
			binary.LittleEndian.PutUint64(pack.out[8:16], math.Float64bits(pw.Mass))
			binary.LittleEndian.PutUint64(pack.out[16:24], pw.WriteStep)

			if !yield(unsafe.Pointer(&pack.out)) {
				return
			}
		}
	}
}
