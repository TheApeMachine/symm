package nomagique

import (
	"github.com/theapemachine/symm/nomagique/types"
)

// Re-exported type aliases and function variables from nomagique/types so that
// the root package retains its historical public surface while the canonical
// implementations live in nomagique/types.
type (
	AtomicStream = types.AtomicStream
	Frame        = types.Frame
	Primitive    = types.Primitive
	Stream       = types.Stream
	Symbol       = types.Symbol
)

const MaxSlots = types.MaxSlots

var (
	Assign           = types.Assign
	Configure        = types.Configure
	FrameFromNamed   = types.FrameFromNamed
	Fork             = types.Fork
	ForkStrict       = types.ForkStrict
	In              = types.In
	Identity         = types.Identity
	Intern           = types.Intern
	Join             = types.Join
	MustIntern       = types.MustIntern
	NewAtomicStream  = types.NewAtomicStream
	NewStream        = types.NewStream
	Out             = types.Out
	Pipe             = types.Pipe
	PrimitiveError   = types.PrimitiveError
	Relay            = types.Relay
	RegisteredSymbols = types.RegisteredSymbols
	State           = types.State
	Step             = types.Step
	SymbolName       = types.SymbolName
	Wire             = types.Wire
)
