package tests

import "github.com/theapemachine/symm/nomagique/core"

// Opaque detects attempts to decode an object during transport. The fixture is
// shared here rather than reimplementing a fake algebra in individual tests.
type Opaque struct{ core.PrimitiveError }

func NewOpaque() *Opaque                           { return &Opaque{} }
func (*Opaque) Next(core.Primitive) core.Primitive { return nil }
func (*Opaque) Read() any                          { panic("transport attempted to decode an opaque Primitive") }
