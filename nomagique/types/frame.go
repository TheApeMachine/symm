package types

import (
	"fmt"
	"iter"
	"math"
	"math/bits"
)

const frameMaskWords = (MaxSlots + frameMaskWordBits - 1) / frameMaskWordBits

/*
Frame is the universal named fact and state representation. Values occupy
interned symbol offsets in contiguous memory, while the bit mask records which
slots are present. A Frame is both the committed state and the step output;
retained estimator values and transient results share the same slots. Err
carries the first validation failure a primitive recorded; a non-nil Err stops
the pipeline.
*/
type Frame struct {
	Mask [frameMaskWords]uint64
	Data [MaxSlots]float64
	Err  error
}

// Get returns the value stored at symbol.
func (frame *Frame) Get(symbol Symbol) (float64, bool) {
	if frame == nil {
		return 0, false
	}

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

// MustGet returns the value stored at symbol or panics.
func (frame Frame) MustGet(symbol Symbol) float64 {
	value, found := frame.Get(symbol)

	if !found {
		panic(fmt.Sprintf("nomagique: frame symbol %s is missing", symbolLabel(symbol)))
	}

	return value
}

// Has reports whether symbol is populated.
func (frame Frame) Has(symbol Symbol) bool {
	_, found := frame.Get(symbol)

	return found
}

// Put writes one slot into the current Frame value.
func (frame *Frame) Put(symbol Symbol, value float64) {
	if frame == nil {
		panic("nomagique: cannot put into a nil Frame")
	}

	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		panic(fmt.Sprintf("nomagique: symbol %d is outside Frame capacity", symbol))
	}

	maskIndex := index >> 6
	frame.Mask[maskIndex] |= uint64(1) << uint(index&63)
	frame.Data[index] = value
}

// Set returns a copied Frame with one slot written.
func (frame Frame) Set(symbol Symbol, value float64) Frame {
	frame.Put(symbol, value)

	return frame
}

// Delete clears one slot.
func (frame *Frame) Delete(symbol Symbol) {
	if frame == nil {
		return
	}

	index := int(symbol)

	if index < 0 || index >= MaxSlots {
		return
	}

	maskIndex := index >> 6
	frame.Mask[maskIndex] &^= uint64(1) << uint(index&63)
	frame.Data[index] = 0
}

// Merge overlays every populated slot from other onto frame.
func (frame *Frame) Merge(other Frame) {
	if frame == nil {
		panic("nomagique: cannot merge into a nil Frame")
	}

	for maskIndex, mask := range other.Mask {
		for remaining := mask; remaining != 0; remaining &= remaining - 1 {
			bit := bits.TrailingZeros64(remaining)
			index := (maskIndex << 6) + bit
			frame.Mask[maskIndex] |= uint64(1) << uint(bit)
			frame.Data[index] = other.Data[index]
		}
	}
}

// Merged returns a copied Frame with other overlaid.
func (frame Frame) Merged(other Frame) Frame {
	frame.Merge(other)

	return frame
}

// Count returns the number of populated slots.
func (frame Frame) Count() int {
	count := 0

	for _, mask := range frame.Mask {
		count += bits.OnesCount64(mask)
	}

	return count
}

// All iterates populated slots in ascending symbol order.
func (frame Frame) All() iter.Seq2[Symbol, float64] {
	return func(yield func(Symbol, float64) bool) {
		for maskIndex, mask := range frame.Mask {
			for remaining := mask; remaining != 0; remaining &= remaining - 1 {
				bit := bits.TrailingZeros64(remaining)
				index := (maskIndex << 6) + bit

				if index >= MaxSlots || !yield(Symbol(index), frame.Data[index]) {
					return
				}
			}
		}
	}
}

// Finite reports whether every populated value is finite.
func (frame Frame) Finite() bool {
	for maskIndex, mask := range frame.Mask {
		for remaining := mask; remaining != 0; remaining &= remaining - 1 {
			bit := bits.TrailingZeros64(remaining)
			value := frame.Data[(maskIndex<<6)+bit]

			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false
			}
		}
	}

	return true
}

// Equal compares exact populated slots and IEEE-754 bit patterns.
func (frame Frame) Equal(other Frame) bool {
	if frame.Mask != other.Mask {
		return false
	}

	for maskIndex, mask := range frame.Mask {
		for remaining := mask; remaining != 0; remaining &= remaining - 1 {
			bit := bits.TrailingZeros64(remaining)
			index := (maskIndex << 6) + bit

			if math.Float64bits(frame.Data[index]) != math.Float64bits(other.Data[index]) {
				return false
			}
		}
	}

	return true
}
