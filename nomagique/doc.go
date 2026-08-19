/*
Package nomagique implements a universal numeric reducer engine.

The thesis is in the name: no magic numbers. Every value is a plain float64 in
an interned slot of a value-type Frame, every computation is a pure Primitive
transition from one Frame state to the next, and every market signal is a
composed numeric unit that sizes, adapts, and scores itself from the data —
never from a hardcoded horizon.

Three contracts hold the whole engine together:

  - Frame is the universal numeric payload and state representation.
  - Primitive is the universal reducer: func(state, input) (nextState, output, err).
  - The input vocabulary in nomagique/types (Quantity, AlphaQuantity,
    BetaQuantity, AlphaPrice, BetaPrice, EventTimeSec, EventTimeNsec, Span)
    is the shared set of numeric slots that lets any preset's output plug into
    any other preset's input.

Composition is expressed through four patterns:

  - Pipe chains primitives so each output feeds the next input.
  - Fork evaluates two reducers against the same input and merges both outputs.
  - Configure wires a control channel: a producer emits a control value (e.g.
    the adaptive window Span), and a consumer receives the original input with
    that value overlaid, with the producer's remaining metrics merged back in.
  - Number is the keyed top-level composer: NewNumber[Key](primitives...) keeps
    one isolated, self-adapting numeric unit per stream key, so each market
    symbol owns its own window, baseline, and event clock.

A consumer signal is then nothing more than a Number plus two boundary
adapters: input conversion (lift a raw market row into the generic vocabulary)
and output push (project the numeric output Frame into a Measurement).
*/
package nomagique
