# Market Simulator

The market simulator is a drop-in replacement for the real Kraken Spot WebSocket connection, and its boundary should also end there.
It is meant to do one thing only: generate relatively realistic raw market data that can be controlled by way of setting, and transitioning between market states.

It was created to allow for testing with the entire system engaged end-to-end.

When tests are written using the market simulator, there are a couple of things to keep in mind:

1. Most important: Do not attempt to write tests that go green by default when testing parts, or the whole of the system. This was specifically created to tease out very hard to find bugs, or logical inconsistencies. New tests going red is almost the entire point, and green should be achieved by improving the system. Another way to think about it: Tests written using the market simulation system define their own expectations, based on common-sense.

2. Do not treat this as unit testing. Tests using the market simulation system should always verify as much as possible. For example, when using this to test a signal, never just assert one metric per test, but assert all metrics for all test cases.

3. Do not write weak assertions. "ShouldBeGreaterThanOrEquals 0" doesn't mean anything. The assertions should always be as precise as possible.

The market simulation system can be a very powerful tool when used correctly, not only deeply testing the mechanics of the system, but also the logical and decision making aspects.
However, this all hinges on putting in serious effort to make the tests rigorous, and not settling for "good enough".