package nomagique

import (
	"fmt"
	"iter"
	"math"
)

const frameMaskWords = (MaxSlots + frameMaskWordBits - 1) / frameMaskWordBits

/*
Frame is the universal numeric payload and state representation. Values occupy
interned symbol offsets in contiguous memory, while the bit mask records which
slots are present.

Frame is intentionally a value type. Reducers receive snapshots by value, mutate
local copies, and return committed snapshots without sharing mutable maps.
*/
type Frame struct {
	Mask [frameMaskWords]uint64
	Data [MaxSlots]float64
}

/*
Get returns the value stored at symbol.
*/
func (frame Frame) Get(symbol Symbol) (float64, bool) {
	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		return 0, false
	}

	maskIndex := index >> 6
	mask := uint64(1) << uint(index&63)

	if frame.Mask[maskIndex]&mask == 0 {
		return 0, false
	}

	return frame.Data[index], true
}

/*
MustGet returns the value stored at symbol or panics. It is useful for tests and
for validated internal paths where absence is a programming error.
*/
func (frame Frame) MustGet(symbol Symbol) float64 {
	value, found := frame.Get(symbol)

	if !found {
		name, named := SymbolName(symbol)

		if named {
			panic(fmt.Sprintf("nomagique: frame symbol %q is missing", name))
		}

		panic(fmt.Sprintf("nomagique: frame symbol %d is missing", symbol))
	}

	return value
}

/*
Has reports whether symbol is populated.
*/
func (frame Frame) Has(symbol Symbol) bool {
	_, found := frame.Get(symbol)

	return found
}

/*
Put writes one slot into the current Frame value. Reducers normally copy their
input state once and use Put on that local copy.
*/
func (frame *Frame) Put(symbol Symbol, value float64) {
	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		panic(fmt.Sprintf("nomagique: symbol %d is outside Frame capacity", symbol))
	}

	maskIndex := index >> 6
	frame.Mask[maskIndex] |= uint64(1) << uint(index&63)
	frame.Data[index] = value
}

/*
Set returns a copied Frame with one slot written. Put is preferable when several
values are written to the same local snapshot.
*/
func (frame Frame) Set(symbol Symbol, value float64) Frame {
	frame.Put(symbol, value)

	return frame
}

/*
Delete clears one slot.
*/
func (frame *Frame) Delete(symbol Symbol) {
	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		return
	}

	maskIndex := index >> 6
	frame.Mask[maskIndex] &^= uint64(1) << uint(index&63)
	frame.Data[index] = 0
}

/*
Merge overlays every populated slot from other onto frame.
*/
func (frame *Frame) Merge(other Frame) {
	for maskIndex, mask := range other.Mask {
		remaining := mask

		for remaining != 0 {
			bit := trailingBit(remaining)
			index := (maskIndex << 6) + bit
			frame.Mask[maskIndex] |= uint64(1) << uint(bit)
			frame.Data[index] = other.Data[index]
			remaining &^= uint64(1) << uint(bit)
		}
	}
}

/*
Merged returns a copied Frame with other overlaid.
*/
func (frame Frame) Merged(other Frame) Frame {
	frame.Merge(other)

	return frame
}

/*
Count returns the number of populated slots.
*/
func (frame Frame) Count() int {
	count := 0

	for _, mask := range frame.Mask {
		count += populationCount(mask)
	}

	return count
}

/*
All iterates populated slots in ascending symbol order without exposing mutable
backing storage.
*/
func (frame Frame) All() iter.Seq2[Symbol, float64] {
	return func(yield func(Symbol, float64) bool) {
		for index := 0; index < MaxSlots; index++ {
			symbol := Symbol(index)
			value, found := frame.Get(symbol)

			if found && !yield(symbol, value) {
				return
			}
		}
	}
}

/*
Finite reports whether every populated value is finite.
*/
func (frame Frame) Finite() bool {
	for _, value := range frame.All() {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}

/*
Equal compares populated slots and their values.
*/
func (frame Frame) Equal(other Frame) bool {
	if frame.Mask != other.Mask {
		return false
	}

	for index := 0; index < MaxSlots; index++ {
		if frame.Data[index] != other.Data[index] {
			return false
		}
	}

	return true
}

func trailingBit(value uint64) int {
	index := 0

	for value&1 == 0 {
		value >>= 1
		index++
	}

	return index
}

func populationCount(value uint64) int {
	count := 0

	for value != 0 {
		value &= value - 1
		count++
	}

	return count
}
