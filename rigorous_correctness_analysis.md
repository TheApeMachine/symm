# Rigorous correctness and consistency analysis

## Scope and normalization

This analysis treats `symm(42).txt`, `nomagique(15).txt`, and `datura(11).txt` as one repository-style system. The bundle export prefixes source lines with display line numbers; those prefixes were removed before parsing. One Go file retained those prefixes after the initial extraction and was corrected only in the analysis copy. After that normalization, all supplied TypeScript/TSX files pass TypeScript syntactic parsing and all supplied Go files pass the Go parser/gofmt parser.

The findings below are face-value statements about the supplied function bodies and contracts. A denominator or mathematical domain is considered defended only when the function itself proves it, or when the supplied type contract makes the invalid state unrepresentable. Caller convention and unstated market-data assumptions are not treated as proofs.

## Finding ledger

| Finding family | Confirmed code sites | Mathematical invariant |
|---|---:|---|
| Empty-sample count division | 2 | `n > 0` |
| Sample-estimator `n - 1` division | 2 | `n >= 2` |
| Other dynamic denominator divisions | 26 | denominator `!= 0`, and often `> 0` by quantity |
| Logarithm domain not established locally | 10 | `x > 0`; for `Log1p`, `x > -1` |
| Square-root radicand vulnerable to cancellation/domain failure | 5 | radicand `>= 0` |
| Stochastic calculations without injected, recorded PRNG state | 4 | same inputs and seed must reproduce the same sample path |
| Quantile index mapping without a complete interpolation contract | 2 | `p in [0,1]`, non-empty sorted sample, explicit estimator |

The ledger contains 51 distinct code sites. Duplicate analyzer matches at identical source expressions were removed. The denominator families remain separate because `n`, `n - 1`, a scale term, and a financial base require different domain statements.

## Empty-sample division

The affected functions divide an aggregate by a sample count without establishing that the count is positive. The arithmetic does not define a mean for an empty set; JavaScript then produces `NaN` or an infinity depending on the numerator, which can continue through later calculations as an ordinary `number`.

The prescribed correction is to reject `n == 0` inside the primitive that computes the statistic. In TypeScript, throw a typed domain error before the division and let callers handle that explicit failure; do not return zero, `NaN`, or a substituted denominator.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `symm/frontend/src/components/terminal/charts.tsx:669` | `<module>` | `(height - 24) / Math.max(positions.length, 1)` | Require the input length to be at least 1 inside this function; raise a typed empty-sample error before evaluating the division. |
| `symm/frontend/src/components/terminal/xray-view.ts:70` | `<module>` | `total / state.length` | Require the input length to be at least 1 inside this function; raise a typed empty-sample error before evaluating the division. |

## Sample-estimator denominator

The affected formulas divide by `n - 1`. That is the sample-estimator denominator and is defined only for `n >= 2`. Checking merely for a non-empty sample is insufficient: `n == 1` still gives a zero denominator.

The prescribed correction is to enforce `n >= 2` in the statistic primitive and use the sample formula consistently:

```text
mean = sum(x_i) / n
sampleVariance = sum((x_i - mean)^2) / (n - 1)
```

Do not silently switch to population variance for a singleton and do not return zero variance as a fallback, because both change the estimator rather than handling an invalid sample.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `symm/frontend/src/components/kernel/inspector.tsx:86` | `<module>` | `index / Math.max(series.length - 1, 1)` | Require n >= 2 at this function boundary and raise a typed insufficient-sample error before computing the estimator. |
| `symm/frontend/src/components/kernel/row.tsx:21` | `<module>` | `index / (history.length - 1)` | Require n >= 2 at this function boundary and raise a typed insufficient-sample error before computing the estimator. |

## Other dynamic denominators

These divisions use denominators such as a scale, dispersion, total weight, price, capital base, or another runtime expression without locally proving the required nonzero/positive domain. The ratio is therefore capable of becoming `NaN` or an infinity, or of changing sign in a quantity whose denominator is supposed to be positive.

The prescribed correction is to evaluate each denominator exactly once, validate the quantity-specific invariant at the function boundary, and propagate a typed domain error. Do not clamp a denominator to an arbitrary epsilon: that converts an undefined statistic into a very large but apparently valid one and directly distorts strategy comparison.

### TypeScript sites

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `symm/frontend/src/components/terminal/charts.tsx:379` | `<module>` | `index / denominator` | Evaluate the denominator once, require it to be nonzero, and return a typed domain error before division if the condition fails. |
| `symm/frontend/src/components/terminal/dashboard-rail.tsx:26` | `<module>` | `decision.proposedNotional / decision.availableCapital` | Require the price/equity/capital base to be strictly positive at this function boundary and return a typed domain error otherwise. |
| `symm/frontend/src/components/terminal/health.tsx:151` | `<module>` | `bar.count / total` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |
| `symm/frontend/src/components/terminal/xray-draw.ts:86` | `<module>` | `(point.x - xRange.min) / xRange.span` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `symm/frontend/src/components/terminal/xray-draw.ts:90` | `<module>` | `(point.y - yRange.min) / yRange.span` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |

### Go sites

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `datura/dmt/cognitive_engine_runtime.go:165` | `buildContextTrainingMutations` | `float64(nextCount) / denominator` | Require a non-empty sample before division and return a typed empty-sample error. |
| `datura/dmt/cognitive_engine_runtime.go:198` | `writeArtifact` | `1.0 / float64(parentWeight.Count+1)` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |
| `nomagique/algorithm/cohort_sample.go:167` | `observe` | `tick.price/symbolState.lastPrice` | Require the price/equity/capital base to be strictly positive at this function boundary and return a typed domain error otherwise. |
| `nomagique/causal/table.go:177` | `backdoorEffect` | `dotTarget / denominator` | Evaluate the denominator once, require it to be nonzero, and return a typed domain error before division if the condition fails. |
| `nomagique/equation/causalstory.go:193` | `shockScore` | `condition / (condition + rungTotal)` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |
| `nomagique/geometry/eigenmode_toroidal.go:526` | `normalizeVec` | `1.0/norm` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/geometry/pga.go:34` | `MotorFromAxisAngle` | `axisZ/norm` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/geometry/pga.go:101` | `Interpolate` | `math.Sin(newHalf) / eucNorm` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/hawkes/bounds.go:121` | `crossBranchFloorFromContext` | `1 / context.SpanSec / float64(context.TotalEvents)` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |
| `nomagique/hawkes/fit_context.go:306` | `branchCeiling` | `1 / math.Sqrt(float64(tune.totalEvents))` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |
| `nomagique/learning/resonance.go:399` | `Learn` | `rm.cfg.GradClip/(norm+1e-12)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:427` | `Learn` | `rm.cfg.GradClip/(norm+1e-12)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:458` | `Learn` | `rm.cfg.GradClip/(norm+1e-12)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:490` | `Learn` | `rm.cfg.GradClip/(norm+1e-12)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:802` | `stateGradients` | `rm.cfg.GradClip/(gradientNorm+1e-12)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:849` | `updatePrecision` | `1.0 / (variance + rm.cfg.PrecisionEps)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:871` | `updatePrecision` | `1.0 / (variance + rm.cfg.PrecisionEps)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/resonance.go:893` | `updatePrecision` | `1.0 / (variance + rm.cfg.PrecisionEps)` | Treat a zero scale/dispersion as an undefined ratio: return a typed domain error before division; do not substitute 0 or an epsilon. |
| `nomagique/learning/rls.go:309` | `observeOnce` | `px[row] / denominator` | Evaluate the denominator once, require it to be nonzero, and return a typed domain error before division if the condition fails. |
| `symm/logic/causal.go:155` | `observe` | `state.ReferencePrice / causal.pending.midPrice` | Require the price/equity/capital base to be strictly positive at this function boundary and return a typed domain error otherwise. |
| `symm/strategy/planner.go:358` | `Decide` | `available / float64(freeTotal)` | Require the aggregate weight/mass to be strictly positive and return a typed domain error when it is not. |

## Logarithm domains

A real-valued `log(x)` requires `x > 0`; `log1p(x)` requires `x > -1`. The supplied bodies contain calls where that domain is not established at the call site. Downstream checks for `NaN` do not repair the calculation because the invalid value has already entered the metric path.

The prescribed correction is domain validation immediately before each logarithm, using the exact domain of the called function, followed by a typed error. Do not apply `abs`, clamp to an epsilon, or drop the observation: each of those silently changes the statistic.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `nomagique/algorithm/streams.go:207` | `hayashiVarianceSum` | `math.Log(samples[index].Value / samples[index-1].Value)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/correlation/hayashi.go:132` | `varianceSum` | `math.Log(samples[index].Value / samples[index-1].Value)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/hawkes/bounds.go:137` | `softplus` | `math.Log1p(math.Exp(value))` | Require x > -1 immediately before Log1p and return a typed domain error otherwise. |
| `nomagique/hawkes/bounds.go:149` | `inverseSoftplus` | `math.Log(math.Expm1(value))` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/hawkes/gradient.go:121` | `eventLogLikelihoodGradient` | `math.Log(lambda)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/hawkes/gradient.go:135` | `eventLogLikelihoodGradient` | `math.Log(lambda)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/mcts/node.go:46` | `SelectBestChild` | `math.Log(float64(n.Visits))` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `nomagique/mcts/search.go:113` | `bestChild` | `math.Log(float64(node.Visits))` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `symm/logic/causal.go:155` | `observe` | `math.Log(state.ReferencePrice / causal.pending.midPrice)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |
| `symm/logic/resonance.go:248` | `learnReturn` | `math.Log(state.ReferencePrice / resonance.pendingMid)` | Require the logarithm argument to be strictly positive immediately before the call and return a typed domain error otherwise. |

## Square-root radicands

The affected square-root inputs are formed by subtractive expressions whose mathematical value is expected to be nonnegative but whose floating-point result can become slightly negative through cancellation. A blanket `sqrt(max(0, r))` is also insufficient because it hides genuinely invalid negative values.

The prescribed correction is a scale-aware two-part check. Let `r` be the computed radicand and `s` be the maximum of `1` and the absolute magnitudes of the terms used to form it. Define `tol = 32 * machineEpsilon * s`. Return a typed domain error when `r < -tol`; otherwise evaluate `sqrt(max(0, r))`. This clamps only representational noise and preserves detection of a real invariant violation.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `nomagique/hawkes/bounds.go:118` | `crossBranchFloorFromContext` | `math.Sqrt(math.Nextafter(1, 2)-1)` | Compute the radicand once; reject values below a scale-aware negative tolerance, then apply `math.Sqrt(math.Max(0, r))` only within that tolerance. |
| `nomagique/hawkes/estimator.go:162` | `logLikelihoodTolerance` | `math.Sqrt(math.Nextafter(1, 2)-1)` | Compute the radicand once; reject values below a scale-aware negative tolerance, then apply `math.Sqrt(math.Max(0, r))` only within that tolerance. |
| `nomagique/learning/rls.go:214` | `rlsCovarianceFloorScale` | `math.Sqrt(math.Nextafter(1, 2) - 1)` | Compute the radicand once; reject values below a scale-aware negative tolerance, then apply `math.Sqrt(math.Max(0, r))` only within that tolerance. |
| `nomagique/statistic/ridge.go:177` | `machineSqrtEpsilon` | `math.Sqrt(math.Nextafter(1, 2) - 1)` | Compute the radicand once; reject values below a scale-aware negative tolerance, then apply `math.Sqrt(math.Max(0, r))` only within that tolerance. |
| `symm/logic/manifold/inject.go:369` | `carrierSpeed` | `math.Sqrt( config.Gamma * (config.Gamma - 1) * specificInternalEnergy, )` | Compute the radicand once; reject values below a scale-aware negative tolerance, then apply `math.Sqrt(math.Max(0, r))` only within that tolerance. |

## Reproducible stochastic calculations

The affected stochastic paths use random state that is not injected and recorded as part of the calculation input. Consequently, identical market data and parameters do not uniquely determine the output, which prevents exact replay of a discovered result.

The prescribed correction is to construct one `*rand.Rand` from an explicit seed at the run boundary, pass that generator into every stochastic primitive, and store the seed in the paper-trading/discovery result metadata. Do not seed inside individual functions and do not use package-global random calls.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `nomagique/mcts/search.go:179` | `simulate` | `rand.Intn(len(actions))` | Replace package/global random access with an injected `*rand.Rand`; create it once from a run seed and record that seed with the result. |
| `symm/kraken/websocket/simulator.go:86` | `Initialize` | `rand.Intn(90)` | Replace package/global random access with an injected `*rand.Rand`; create it once from a run seed and record that seed with the result. |
| `symm/kraken/websocket/simulator.go:92` | `Initialize` | `rand.Intn(90)` | Replace package/global random access with an injected `*rand.Rand`; create it once from a run seed and record that seed with the result. |
| `symm/kraken/websocket/simulator.go:98` | `Initialize` | `rand.Intn(360)` | Replace package/global random access with an injected `*rand.Rand`; create it once from a run seed and record that seed with the result. |

## Quantile mapping

The affected quantile implementations map a probability directly to an integer index without a complete endpoint and interpolation contract. This creates estimator-dependent bias, discontinuities, and possible endpoint errors; the value can also depend on whether the input was correctly sorted.

The prescribed correction is one canonical estimator everywhere: validate a non-empty sample and `0 <= p <= 1`, copy and numerically sort the data, then use linear interpolation on the zero-based position `h = (n - 1) * p`:

```text
j = floor(h)
g = h - j
Q(p) = x[j]                         when j = n - 1
Q(p) = x[j] + g * (x[j+1] - x[j]) otherwise
```

This defines the endpoints exactly: `Q(0) = min(x)` and `Q(1) = max(x)`.

| Location | Function | Evidence | Required correction |
|---|---|---|---|
| `nomagique/causal/table.go:235` | `percentile` | `func (nodeTable nodeTable) percentile(node int, percentile float64) (float64, error) { values, err := nodeTable.column(node) if err != nil { return 0, err } if len(values) == 0 { return 0, errnie.Error(errnie.Err( errnie` | Replace direct integer indexing with the stated `(n - 1) * p` linear-interpolation estimator, after validating p and numerically sorting a copy. |
| `nomagique/statistic/quantile.go:28` | `NewQuantile` | `func NewQuantile(configs ...QuantileConfig) *Quantile { config := QuantileConfig{ Percentile: 0.5, Kind: stat.LinInterp, } if len(configs) > 0 { config = configs[0] } return &Quantile{ config: config, } }` | Replace direct integer indexing with the stated `(n - 1) * p` linear-interpolation estimator, after validating p and numerically sorting a copy. |

## Cross-file consistency

The explicit cross-file checks compared strongly matched contracts across the three bundles and both languages, resolving TypeScript aliases, Go named underlying types, and Go `time.Time` JSON string representation before comparison. No hard field-type or requiredness contradiction remained after that resolution. No conflicting explicit enum value set, same-named semantic numeric constant, or unit suffix representation was found among those strongly matched contracts.

Same-named variance, volatility, and covariance functions were checked for population (`n`) versus sample (`n - 1`) denominator disagreement; no direct same-name contradiction was found. This does not make the individual `n - 1` domain omissions valid—the sites above still require `n >= 2`.

## Checks that did not produce a concrete finding

Within the supplied code and the static patterns checked, there was no concrete basis-point `/100` versus `/10000` error, additive aggregation of simple returns where compounding was explicitly required, rolling-window end-bound omission, unstable softmax implementation, inconsistent EMA alpha domain, same-name cross-bundle constant/enum contradiction, or type-proven numeric array using JavaScript's no-comparator `.sort()`. The broader text scan found no-comparator sort calls, but the TypeScript checker could not establish a numeric element type for any of them, so they are not reported as mathematical defects.

These are bounded negative results from the supplied source, not assumptions about code that was not present.

## Canonical correction policy

The mathematically coherent resolution across these files is to make domain validity part of every primitive's contract: reject invalid samples and denominators explicitly, never manufacture a numeric fallback for an undefined statistic, use one shared quantile implementation, use scale-aware tolerance only for proven floating-point cancellation, and make stochastic state an explicit recorded input. That preserves the current strategy-discovery direction while ensuring that metrics and experiments remain interpretable and exactly replayable.