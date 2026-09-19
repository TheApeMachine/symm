package core

import "github.com/krakenfx/api-go/v2/pkg/decimal"

/*
Constants are the only values in nomagique that are not required
to be derived, or adaptive. It is not allowed to make them arbitrary
just to escape the difficulty of "nomagique" which of course states
clearly in its name: no magic. Each constant must be justified
and its presence, plus reasoning and value must be well documented.
*/
const (
	// Unit is the identity element for multiplication, as well as
	// the physical unit (in which case we think of it as 1 simulation step).
	Unit = 1.0

	// Epsilon is the machine epsilon for IEEE 754 double precision (64-bit)
	// floating point numbers (2^-52). It represents the upper bound on the
	// relative approximation error due to rounding, defining the physical
	// representation boundary below which differences are non-distinguishable.
	Epsilon = 2.220446049250313e-16
)

/*
ZeroDecimal is the zero value for decimal.Decimal, used when needed to avoid
initializing a new decimal.Decimal.
*/
var ZeroDecimal = decimal.NewFromInt64(0)
