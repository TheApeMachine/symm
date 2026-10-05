package cognition

import (
	"bytes"
	"encoding/binary"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Train records every contiguous token span of one context under one class.

A span is a run of length-framed timesteps when the whole context is framed,
otherwise a run of 8-byte tokens when the context is aligned that way,
otherwise a run of NUL, slash, or underscore separated tokens. The span
count is the token count of the context that arrived.
*/
type Train struct {
	*core.PrimitiveError
	memory *Associate
}

func NewTrain(memory *Associate) *Train {
	return &Train{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Train) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var text data.Map[string]

			for pointer := range adapter.Next(data.NewValue(data.NewLiteral("context", "class"))) {
				text = *(*data.Map[string])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			context, contextOK := text.Values["context"]
			class, classOK := text.Values["class"]

			if !contextOK || !classOK || context == "" || class == "" {
				op.Error(core.ErrDomain)
				return
			}

			var numbers data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(data.NewMap(
				"feedback", "feedback",
				"graded", "graded",
			))) {
				numbers = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			feedback, feedbackOK := numbers.Values["feedback"]
			graded, gradedOK := numbers.Values["graded"]

			if !feedbackOK || !gradedOK {
				op.Error(core.ErrNotHeld)
				return
			}

			contextBytes := []byte(context)
			framed := len(contextBytes) >= 12
			var starts []int
			var ends []int
			offset := 0

			for framed && offset < len(contextBytes) {
				if offset+4 > len(contextBytes) {
					framed = false
					break
				}

				count := int(binary.BigEndian.Uint32(contextBytes[offset : offset+4]))

				if count == 0 {
					framed = false
					break
				}

				size := 4 + count*8

				if offset+size > len(contextBytes) {
					framed = false
					break
				}

				starts = append(starts, offset)
				ends = append(ends, offset+size)
				offset += size
			}

			if framed && (offset != len(contextBytes) || len(starts) == 0) {
				framed = false
			}

			if !framed {
				starts = nil
				ends = nil
			}

			if len(starts) == 0 && len(contextBytes) >= 8 && len(contextBytes)%8 == 0 {
				for token := 0; token < len(contextBytes); token += 8 {
					starts = append(starts, token)
					ends = append(ends, token+8)
				}
			}

			delim := byte(0)
			delimited := false

			if len(starts) == 0 && bytes.Contains(contextBytes, []byte{0}) {
				delim = 0
				delimited = true
			}

			if len(starts) == 0 && !delimited && bytes.Contains(contextBytes, []byte{'/'}) {
				delim = '/'
				delimited = true
			}

			if len(starts) == 0 && !delimited && bytes.Contains(contextBytes, []byte{'_'}) {
				delim = '_'
				delimited = true
			}

			if delimited {
				start := 0

				for index := 0; index < len(contextBytes); index++ {
					if contextBytes[index] != delim {
						continue
					}

					if index > start {
						starts = append(starts, start)
						ends = append(ends, index)
					}

					start = index + 1
				}

				if start < len(contextBytes) {
					starts = append(starts, start)
					ends = append(ends, len(contextBytes))
				}
			}

			if len(starts) == 0 {
				starts = []int{0}
				ends = []int{len(contextBytes)}
			}

			bridgeState := data.NewState(data.NewMap())
			bridge := data.NewAdapter(nil, bridgeState)
			issued := data.NewOutputMap()
			issued.Values["feedback"] = feedback
			issued.Values["graded"] = graded

			for range bridge.Next(data.NewValue(issued)) {
			}

			if err := bridge.Error(); err != nil {
				op.Error(err)
				return
			}

			for start := 0; start < len(starts); start++ {
				for end := start + 1; end <= len(starts); end++ {
					label := data.NewTextMap()
					label.Values["class"] = class
					label.Values["context"] = string(contextBytes[starts[start]:ends[end-1]])

					for range bridge.Next(data.NewValue(label)) {
					}

					if err := bridge.Error(); err != nil {
						op.Error(err)
						return
					}

					for range op.memory.Next(data.NewValue(bridge)) {
					}

					if err := op.memory.Error(); err != nil {
						op.Error(err)
						return
					}

					if err := bridge.Error(); err != nil {
						op.Error(err)
						return
					}
				}
			}

			var published data.Map[float64]

			for pointer := range bridge.Next(data.NewValue(data.NewMap(
				"records", "records",
				"span", "span",
			))) {
				published = *(*data.Map[float64])(pointer)
			}

			if err := bridge.Error(); err != nil {
				op.Error(err)
				return
			}

			forwarded := data.NewOutputMap()
			forwarded.Values["records"] = published.Values["records"]
			forwarded.Values["span"] = published.Values["span"]

			for range adapter.Next(data.NewValue(forwarded)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
