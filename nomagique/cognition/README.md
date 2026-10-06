# cognition

Associative memory as primitives. Each one has a constructor, embeds
`*core.PrimitiveError`, and implements `Next`. `Next` reads and writes a
`data.Adapter`.

`Associate` records one context against one class in an immutable radix trie.
Basin keys are `b/<context>/<class>`. Sensory keys are `s/<context>`. Key `0`
is the observation clock and key `1` is the longest stored token span. A
record is 24 bytes: little-endian count, probability bits, and write step.

`Recall` reads the class back. An exact basin hit wins. Failing that, the
longest stored prefix is tried, then the longest stored suffix. Equal leading
masses abstain.

`Train` records every contiguous token span of one context, so a later suffix
can match it. Spans are length-framed timesteps when more than one frame fills
the context, otherwise 8-byte tokens when the context is longer than one token
and aligned that way, otherwise NUL, slash, or underscore separated tokens.

`Census` publishes `records`, `span`, and the number of distinct basin keys
introduced for each class. `Snapshot` publishes the trie as text under `model`
in the `cognition/association/1` gob layout. `Restore` loads that text into an
empty trie. `Export` publishes the dashboard tree as JSON text under `tree`.

Text keys are `context`, `class`, `winner`, `runner_up`, `model`, and `tree`.
Number keys are `feedback`, `graded`, `probability`, `count`, `confidence`,
`contrast`, `support`, `ambiguity`, `surprisal`, `records`, and `span`.
A graded association starts at the center of the unit interval. Zero graded
feedback records the sensory transition and does not reinforce the basin.
