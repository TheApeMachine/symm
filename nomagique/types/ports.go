package types

/*
Structural ports are the canonical local coordinate system a primitive reads
and writes. They carry no domain meaning: PortA and PortB are the first and
second operands, PortX is the focal value, and PortResult is the operation's
output. Composition binds outer named facts to these ports with Wire, so a
primitive never has to know whether PortA is a price, a volume, or a duration.

The interned names ("left", "right", "value", "result") are the historical
slot names; they are aliased here so every primitive package can reference the
same structural ports without importing the calculus package.
*/
var (
	PortA      = MustIntern("left")
	PortB      = MustIntern("right")
	PortX      = MustIntern("value")
	PortResult = MustIntern("result")
)
