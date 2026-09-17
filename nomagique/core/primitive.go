package core

import (
	"iter"
	"unsafe"
)

/*
Primitive is the interface that all nomagique types must implement. Consider nomagique
a streaming, composable algebra. Each type must constrain itself to the absolute most
minimal implementation of one transformation. This can essentially manifest in two ways:

1. Implementation of a new transformation.
2. Composition of existing Primitive types.

THE RULES:

(Please note that due to A.I. agent misunderstanding, the rules are often still broken
in the current package, this does not mean you are allowed to break them!)

- A Primitive is a constructor, a Next method, and *PrimitiveError composed onto the type.
- A Primitive does one thing and does it well. It is a transformation.
- A Primitive is forbidden to contain any helper methods, that is a severe anti-pattern.
- A Primitive is allowed to implement interfaces from the core package.
- The core package may not be arbitrarily extended.
- When you run into some issue or missing functionality, the answer is always: another Primitive.
- You may not escape the composable algebra, but you may extend it.

Before creating a new Primitive always first consider:

1. Does what I need already exist as a Primitive?

Primitive types should be placed in a sub-package that generally represents
the Primitive's most canonical domain or discipline.
To avoid import cycles, a composition can always be promoted to an equation.
The equation sub-package may only contain composition, no implementation code.
The algo package has the same rules as equation, with one additional constraint,
algo Primitives may only be well-known, named algorithms.

2. Does what I need exist if I make an existing Primitive more flexible?

This can be done by expanding the constructor, or potentially also by wrapping
things in another Primitive to basically act as a decorator, altering behavior.

3. Does what I need exist if I compose multiple existing Primitives?

This is really always the goal, and the idea behind it is that we can validate
the code once, and then always confidently use it, while also keeping an eye
on the other principle: never using magic numbers, or otherwise non-derived
values.

You are not implementing the end goal in nomagique, you are just implementing the
Primitives you need to build your pipelines somewhere else.
*/
type Primitive interface {
	Next(iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer]
	Error(...error) error
}
