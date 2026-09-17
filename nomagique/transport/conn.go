package transport

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Conn makes a primitive pipeline addressable and connectable. Its data path
streams input through the member pipeline, then forwards that output to the
connected peer.
*/
type Conn[T comparable] struct {
	*core.PrimitiveError
	identity T
	member   core.Primitive
	peer     core.Primitive
}

func NewConn[T comparable](member core.Primitive) *Conn[T] {
	return &Conn[T]{
		PrimitiveError: core.NewPrimitiveError(),
		member:         member,
	}
}

func (conn *Conn[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if conn.member != nil {
			defer func() { conn.Error(conn.member.Error()) }()
		}

		if conn.peer != nil {
			defer func() { conn.Error(conn.peer.Error()) }()
		}

		stream := in

		if conn.member != nil {
			stream = conn.member.Next(in)
		}

		if conn.peer != nil {
			stream = conn.peer.Next(stream)
		}

		for item := range stream {
			if !yield(item) {
				return
			}
		}
	}
}

func (conn *Conn[T]) Identity() T {
	return conn.identity
}

func (conn *Conn[T]) Identify(identity T) core.Identifiable[T] {
	conn.identity = identity
	return conn
}

func (conn *Conn[T]) Connect(primitive core.Primitive) {
	conn.peer = primitive
}
