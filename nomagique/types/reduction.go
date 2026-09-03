package types

/*
Reduction is a pure fold over a collection of Number samples.
Reductions do not retain buffers across ticks; they fold the buffer passed to them.
*/
type Reduction func([]Number) Number
