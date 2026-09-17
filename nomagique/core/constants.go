package core

/*
Constants are the only values in nomagique that are not required
to be derived, or adaptive. It is not allowed to make them arbitrary
just to escape the difficulty of "nomagique" which of course states
clearly in its name: no magic. Each constant must be justified
and its presence, plus reasoning and value must be well documented.
*/
const (
	// Unit is the identity element for multiplication, as well as
	// the physical unit (in which case we think of it as 1 simulation step)
	Unit = 1.0
)
