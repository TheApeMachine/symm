/*
Package types retains descriptive boundary metadata and migration aliases for
the universal nomagique engine types.

The former Input, Output, IO, Value, and pointer-backed Map lifecycle is
intentionally not reproduced: reducer code should use nomagique.Frame,
nomagique.Primitive, and nomagique.Stream directly. Keeping those mutable
interfaces as compatibility wrappers would preserve the architecture this
migration replaces.
*/
package types
