# review

Please review the project according to the guidelines in AGENTS.md and the following additional rules.

## Correctness

* Look for anything that is either incorrect, questionable, or otherwise seems like a less than optimal method to achieve the goal of a section of code.
* Look for anything that relies on magic numbers, static values (including static time windows), or otherwise non-dynamic/non-adaptive mechanics.

## Performance

* Look for any opportunity to improve the performance of the code.

## Complexity

* Look for any signs of over-engineering, especially where the code makes too many hops across "helper" methods.
* Look for bad compositional patterns that do not follow the example in AGENTS.md and weird patterns where methods/functions only exist to call other methods/functions
* Look for loose functions that should be proper methods on composed types.
* Look for any and all `if` statements that could be replaced with things like `max` `min` or other built-in/standard library methods.
* Look for overly defensive patterns where there is more validation code than actual useful implementation.

The list above is not exhaustive and we rely on you to also use your own best judgement to highlight additional bad practices.