package data

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

type ActionType uint8

const (
	NOOP ActionType = iota
	READ
	WRITE
)

type Message struct {
	*core.PrimitiveError
	Action  ActionType
	address string
	Address string
	Key     string
	Value   core.Primitive
}

func NewMessage(
	action ActionType, address, key string, value core.Primitive,
) *Message {
	return &Message{
		PrimitiveError: core.NewPrimitiveError(),
		Action:         action,
		Address:        address,
		Key:            key,
		Value:          value,
	}
}

func (message *Message) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			msg := message
			yield(unsafe.Pointer(msg))
			return
		}

		for ptr := range in {
			if !yield(ptr) {
				return
			}
		}
	}
}
