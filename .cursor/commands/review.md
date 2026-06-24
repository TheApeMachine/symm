# review

Please review the project according to the guidelines in AGENTS.md and the following additional rules.

> NOTE! We are no longer using the pure nomagique pipelines, since you couldn't get them right.

## Correctness

* Look for anything that is either incorrect, questionable, or otherwise seems like a less than optimal method to achieve the goal of a section of code.
* Look for anything that relies on magic numbers, static values (including static time windows), or otherwise non-dynamic/non-adaptive mechanics.
* Look for any places requiring "warmup" or other delayed processing. This should not happen, any windowing should automatically grow/shrink scale, based on derived values (such as timestamps/time between measurements, per symbol of course). Things like mean values or requiring baselines should use the first value directly (the mean of the first value is the value), and grow from there. The max size, for windows, might be a crosscutting mean across symbols such that the windows across all symbols are somewhat normalized, I am not sure yet.

## Performance

* Look for any opportunity to improve the performance of the code.

## Complexity

* Look for any signs of over-engineering, especially where the code makes too many hops across "helper" methods.
* Look for bad compositional patterns that do not follow the example in AGENTS.md and weird patterns where methods/functions only exist to call other methods/functions
* Look for loose functions that should be proper methods on composed types.
* Look for any and all `if` statements that could be replaced with things like `max` `min` or other built-in/standard library methods.
* Look for overly defensive patterns where there is more validation code than actual useful implementation.
* This is a mono-typed system, meaning that there should be no DTOs, or Models, besides the Artifact type. The same Artifact should be sent as binary WebSocket message to the front-end, and the frontend should use those Artifact directly, and not try to build new types from them.

The list above is not exhaustive and we rely on you to also use your own best judgement to highlight additional bad practices.

Please do not add severity or priority or any other advise about the order of implementation. Each point should be treated as equally important. Advise about implementation, correctness, etc. is appreciated.