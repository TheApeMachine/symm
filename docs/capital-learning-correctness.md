# Capital learning correctness follow-up

All five issues from the supplied architecture review are implemented on the existing owners and numerical Model. This report supersedes the earlier report's pooled-capital-evidence and triggering-market WAIT semantics.

## Contracts

1. **Unrealized allocations abort.** One atomic immutable AllocationReceipt connects asynchronous dispatch, admission and broker lifecycle facts to the serialized AccountTeacher. Missing authority, invalid inventory/action/quantity, stale/repriced/cash-refused claims, queue refusal, venue failure and zero-fill termination release the pending Model ticket without adding a sample, aging evidence or teaching WAIT. A submitted acknowledgement alone cannot resolve a buy; a positive cumulative fill can. Partial fills remain realized if the remainder fails or is cancelled. Virtual allocation uses its actual surviving-depth fill to report the same facts. Duplicate terminal facts cannot resolve a ticket twice. An existing active lot now returns a typed refusal instead of broker success.
2. **Recall owns sampling context.** PriorReading carries requested ContextLength beside matched Depth and Pending. SamplingVariance uses that effective path for the existing specificity-debt rule, including per-candidate custom recall. Inconsistent metadata fails visibly. The sampling mechanism is still an empirical Gaussian approximation, not a calibrated posterior.
3. **Allocation evidence transfers.** CapitalKnowledge normalizes action identity to kind/power. It binds source-global and source-symbol scopes under one ticket; candidate and execution identities retain their real symbol. Sparse symbols can consume structurally matching global allocation evidence. A supported fresh symbol exception can specialize.
4. **Teacher sources remain separate.** Virtual and actual priors never share samples, support or moments. Each source selects global or symbol evidence using retained EvidenceAuthority times Maturity, requiring defined variance when the broader reading has it. Actual replaces the selected virtual reading only when its retained evidence satisfies the same comparison. This is source selection, not blending, residual learning or a claim of independence. Sparse actual evidence may still defer to virtual evidence. Both sources age on the existing capital resolution clock.
5. **WAIT has a capital clock.** WAIT ends at the earliest remaining current-candidate expiry or measured held-exposure horizon. With neither, the observed shared-account mark interval supplies its duration. No positive measured duration means no window is issued. A selected buy retains its frozen candidate horizon. The decision journal and operator view expose the basis. The triggering market has no special horizon.

The issue-time state, baseline and clock remain frozen when execution arrives. Reward includes decision-to-execution latency and subsequent account economics; it is not a per-fill counterfactual P&L attribution.

## History and operator visibility

Warmup validates source, causal identity, funding, units and temporal ordering. A buy label without confirmed execution is excluded and counted; historical WAIT labels need no execution receipt. An aborted ticket cannot be resurrected by a later label. Reconstruction trains source-separated global/symbol evidence without restoring live account authority.

The capital panel exposes selected source/scope, separate virtual and actual readings, full requested context length, pending execution state, abort counts, execution details and horizon basis. Candidate review exposes later execution/abort facts separately from issue-time inputs.

## Live verification

The freshly built paper backend listened on 127.0.0.1:8766 using a separate journal and position-store directory. The verification frontend used port 3001 and VITE_SYMM_WS_URL=ws://127.0.0.1:8766/ws. Native Chrome inspection verified the shared-capital panel, separate source support, requested context length and actual account view against that backend. Browser-provider control was unavailable; the enabled native Chrome UI supplied the visual check.

The backend started at 2026-09-06 02:12:22 Europe/Rome. The captured HTTP snapshot at 02:14:05 contains 135,460 observations, 448,420 local resolutions and 468 spot symbols. Its selected capital reading contains 148 virtual samples; the alternative actual reading contains 18 samples. It does not contain a summed 166-sample reading. The actual teacher was observing WAIT using a measured 3.039205-second account observation interval. A virtual allocation was confirmed filled under its candidate horizon. Skill remained in learning mode and dispatched zero orders.

The stopped journal contains 28 actual WAIT resolutions, 177 virtual resolutions and four virtual aborts. Every resolved buy has fill confirmation. The main backend on 8765 was not stopped by this verification; a later read showed the updated evidence schema and 106 old allocation labels excluded for lack of fill confirmation. Verification services were stopped after capture. No actual-account buy refusal or fill was manufactured live; deterministic dispatch, broker lifecycle, teacher, replay and race tests cover those transitions.

## Performance

CapitalKnowledge.Reading: 556.9 ns/op, 0 B/op, 0 allocations/op. Dual-scope Issue plus Abort: 268.4 ns/op, 80 B/op, 1 allocation/op. The 64-claim arbitration fixture measures 75,859 ns/op, 79,394 B/op and 369 allocations/op. This is higher than the previous report's 34,153 ns/op as arbitration now reads and records four evidence alternatives across two sources. It is a material per-arbitration cost, not a full-universe throughput claim. The immutable execution receipt is benchmarked through the real lifecycle feedback path.

## Files changed for this follow-up

- [broker/desk.go](../broker/desk.go)
- [broker/desk_test.go](../broker/desk_test.go)
- [cmd/execution_feedback.go](../cmd/execution_feedback.go)
- [cmd/execution_feedback_test.go](../cmd/execution_feedback_test.go)
- [cmd/learning_admission.go](../cmd/learning_admission.go)
- [cmd/learning_desk.go](../cmd/learning_desk.go)
- [cmd/learning_desk_test.go](../cmd/learning_desk_test.go)
- [cmd/root.go](../cmd/root.go)
- [hindsight/allocation.go](../hindsight/allocation.go)
- [hindsight/learning.go](../hindsight/learning.go)
- [nomagique/learning/model.go](../nomagique/learning/model.go)
- [nomagique/learning/model_test.go](../nomagique/learning/model_test.go)
- [nomagique/learning/prior.go](../nomagique/learning/prior.go)
- [nomagique/learning/prior_test.go](../nomagique/learning/prior_test.go)
- [nomagique/learning/selection.go](../nomagique/learning/selection.go)
- [nomagique/learning/selection_test.go](../nomagique/learning/selection_test.go)
- [strategy/account_teacher.go](../strategy/account_teacher.go)
- [strategy/account_teacher_test.go](../strategy/account_teacher_test.go)
- [strategy/allocation_receipt.go](../strategy/allocation_receipt.go)
- [strategy/allocation_receipt_test.go](../strategy/allocation_receipt_test.go)
- [strategy/capital.go](../strategy/capital.go)
- [strategy/capital_test.go](../strategy/capital_test.go)
- [strategy/capital_knowledge.go](../strategy/capital_knowledge.go)
- [strategy/capital_knowledge_test.go](../strategy/capital_knowledge_test.go)
- [strategy/capital_view.go](../strategy/capital_view.go)
- [strategy/capital_warmup.go](../strategy/capital_warmup.go)
- [strategy/capital_warmup_test.go](../strategy/capital_warmup_test.go)
- [strategy/execution.go](../strategy/execution.go)
- [strategy/inspector.go](../strategy/inspector.go)
- [strategy/portfolio.go](../strategy/portfolio.go)
- [strategy/portfolio_test.go](../strategy/portfolio_test.go)
- [frontend/src/components/learning/state.ts](../frontend/src/components/learning/state.ts)
- [frontend/src/components/learning/capital-panel.tsx](../frontend/src/components/learning/capital-panel.tsx)
- [frontend/src/components/learning/capital-panel.test.tsx](../frontend/src/components/learning/capital-panel.test.tsx)
- [frontend/src/components/learning/knowledge-panel.tsx](../frontend/src/components/learning/knowledge-panel.tsx)
- [frontend/src/components/learning/candidate-review.tsx](../frontend/src/components/learning/candidate-review.tsx)
- [docs/learning-architecture-refinement.md](../docs/learning-architecture-refinement.md)

The existing startup/index work in store files is described in the earlier architecture report. Concurrent edits to Makefile, README, .gitignore, balance components and image files are outside this follow-up.

## Exact verification commands and stdout

Go commands ran in the repository root; pnpm commands ran in frontend. Files preserve unmodified command stdout; stderr is separate. The earlier deliberately failing regression is retained as refusal-before.stdout and refusal-before.stderr, proving that the old teacher resolved an unexecuted allocation. Final relevant suites passed.

### targeted

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -p 1 ./nomagique/learning ./strategy ./cmd ./broker
```

```text
ok  	github.com/theapemachine/symm/nomagique/learning	0.741s
ok  	github.com/theapemachine/symm/strategy	1.078s
ok  	github.com/theapemachine/symm/cmd	0.411s
ok  	github.com/theapemachine/symm/broker	4.532s
```

Stderr:

```text
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/broker.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

### strategy

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -p 1 ./strategy
```

```text
ok  	github.com/theapemachine/symm/strategy	1.288s
```

Stderr:

```text
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

### go-tests

```sh
make test-go
```

```text
go test -ldflags=-checklinkname=0 -p 1 -timeout 30m ./...
?   	github.com/theapemachine/symm	[no test files]
ok  	github.com/theapemachine/symm/broker	4.157s
ok  	github.com/theapemachine/symm/cmd	0.438s
ok  	github.com/theapemachine/symm/cmd/hindsight_export	0.442s
?   	github.com/theapemachine/symm/cmd/hindsight_probe	[no test files]
ok  	github.com/theapemachine/symm/hindsight	0.326s
ok  	github.com/theapemachine/symm/kraken	(cached)
ok  	github.com/theapemachine/symm/kraken/websocket	(cached)
ok  	github.com/theapemachine/symm/logic/category	(cached)
ok  	github.com/theapemachine/symm/logic/cognition	(cached)
ok  	github.com/theapemachine/symm/logic/manifold	(cached)
ok  	github.com/theapemachine/symm/logic/resonance	(cached)
ok  	github.com/theapemachine/symm/nomagique	(cached)
ok  	github.com/theapemachine/symm/nomagique/adaptive	(cached)
ok  	github.com/theapemachine/symm/nomagique/algo	(cached)
ok  	github.com/theapemachine/symm/nomagique/calculus	(cached)
ok  	github.com/theapemachine/symm/nomagique/causal	(cached)
ok  	github.com/theapemachine/symm/nomagique/correlation	(cached)
ok  	github.com/theapemachine/symm/nomagique/data	(cached)
ok  	github.com/theapemachine/symm/nomagique/distribution	(cached)
ok  	github.com/theapemachine/symm/nomagique/equation	(cached)
?   	github.com/theapemachine/symm/nomagique/geometry	[no test files]
ok  	github.com/theapemachine/symm/nomagique/learning	0.244s
?   	github.com/theapemachine/symm/nomagique/logic	[no test files]
ok  	github.com/theapemachine/symm/nomagique/network	(cached)
ok  	github.com/theapemachine/symm/nomagique/physics/sensorium	(cached)
ok  	github.com/theapemachine/symm/nomagique/physics/sensorium/metallibgen	(cached)
ok  	github.com/theapemachine/symm/nomagique/probability	(cached)
ok  	github.com/theapemachine/symm/nomagique/relation	(cached)
ok  	github.com/theapemachine/symm/nomagique/runtime	(cached)
ok  	github.com/theapemachine/symm/nomagique/statistic	(cached)
ok  	github.com/theapemachine/symm/nomagique/statistic/hawkes	(cached)
ok  	github.com/theapemachine/symm/nomagique/store	(cached)
?   	github.com/theapemachine/symm/nomagique/temporal	[no test files]
ok  	github.com/theapemachine/symm/nomagique/transport	(cached)
ok  	github.com/theapemachine/symm/nomagique/types	(cached)
?   	github.com/theapemachine/symm/nomagique/utils	[no test files]
ok  	github.com/theapemachine/symm/nomagique/vector	(cached)
ok  	github.com/theapemachine/symm/signal	(cached)
ok  	github.com/theapemachine/symm/signal/correlation	(cached)
ok  	github.com/theapemachine/symm/signal/cvd	(cached)
ok  	github.com/theapemachine/symm/signal/depthflow	(cached)
ok  	github.com/theapemachine/symm/signal/derivatives	(cached)
ok  	github.com/theapemachine/symm/signal/hawkes	(cached)
ok  	github.com/theapemachine/symm/signal/leadlag	(cached)
ok  	github.com/theapemachine/symm/signal/liquidity	(cached)
ok  	github.com/theapemachine/symm/signal/morphology	(cached)
ok  	github.com/theapemachine/symm/signal/pumpdump	(cached)
ok  	github.com/theapemachine/symm/signal/sentiment	(cached)
ok  	github.com/theapemachine/symm/signal/toxicity	(cached)
ok  	github.com/theapemachine/symm/store	0.530s
ok  	github.com/theapemachine/symm/strategy	0.948s
ok  	github.com/theapemachine/symm/system	(cached)
ok  	github.com/theapemachine/symm/telemetry	(cached)
?   	github.com/theapemachine/symm/telemetry/generated/telemetry	[no test files]
ok  	github.com/theapemachine/symm/tests/market	(cached)
ok  	github.com/theapemachine/symm/tools/metriclineage	1.154s
ok  	github.com/theapemachine/symm/tools/metricmap	(cached)
ok  	github.com/theapemachine/symm/types	(cached)
ok  	github.com/theapemachine/symm/ui	0.292s
?   	github.com/theapemachine/symm/utils	[no test files]
```

Stderr:

```text
# github.com/theapemachine/symm/broker.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd/hindsight_export.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/kraken/websocket.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/category.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/cognition.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/manifold.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/resonance.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/nomagique/physics/sensorium.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/nomagique/runtime.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/correlation.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/cvd.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/depthflow.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/derivatives.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/hawkes.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/leadlag.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/liquidity.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/morphology.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/pumpdump.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/sentiment.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/toxicity.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/system.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/types.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/ui.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

### go-race

```sh
make test-race
```

```text
go test -ldflags=-checklinkname=0 -p 1 -timeout 30m -race ./...
?   	github.com/theapemachine/symm	[no test files]
ok  	github.com/theapemachine/symm/broker	8.023s
ok  	github.com/theapemachine/symm/cmd	1.527s
ok  	github.com/theapemachine/symm/cmd/hindsight_export	1.433s
?   	github.com/theapemachine/symm/cmd/hindsight_probe	[no test files]
ok  	github.com/theapemachine/symm/hindsight	1.322s
ok  	github.com/theapemachine/symm/kraken	(cached)
ok  	github.com/theapemachine/symm/kraken/websocket	(cached)
ok  	github.com/theapemachine/symm/logic/category	(cached)
ok  	github.com/theapemachine/symm/logic/cognition	(cached)
ok  	github.com/theapemachine/symm/logic/manifold	(cached)
ok  	github.com/theapemachine/symm/logic/resonance	(cached)
ok  	github.com/theapemachine/symm/nomagique	(cached)
ok  	github.com/theapemachine/symm/nomagique/adaptive	(cached)
ok  	github.com/theapemachine/symm/nomagique/algo	(cached)
ok  	github.com/theapemachine/symm/nomagique/calculus	(cached)
ok  	github.com/theapemachine/symm/nomagique/causal	(cached)
ok  	github.com/theapemachine/symm/nomagique/correlation	(cached)
ok  	github.com/theapemachine/symm/nomagique/data	(cached)
ok  	github.com/theapemachine/symm/nomagique/distribution	(cached)
ok  	github.com/theapemachine/symm/nomagique/equation	(cached)
?   	github.com/theapemachine/symm/nomagique/geometry	[no test files]
ok  	github.com/theapemachine/symm/nomagique/learning	1.765s
?   	github.com/theapemachine/symm/nomagique/logic	[no test files]
ok  	github.com/theapemachine/symm/nomagique/network	(cached)
ok  	github.com/theapemachine/symm/nomagique/physics/sensorium	(cached)
ok  	github.com/theapemachine/symm/nomagique/physics/sensorium/metallibgen	(cached)
ok  	github.com/theapemachine/symm/nomagique/probability	(cached)
ok  	github.com/theapemachine/symm/nomagique/relation	(cached)
ok  	github.com/theapemachine/symm/nomagique/runtime	(cached)
ok  	github.com/theapemachine/symm/nomagique/statistic	(cached)
ok  	github.com/theapemachine/symm/nomagique/statistic/hawkes	(cached)
ok  	github.com/theapemachine/symm/nomagique/store	(cached)
?   	github.com/theapemachine/symm/nomagique/temporal	[no test files]
ok  	github.com/theapemachine/symm/nomagique/transport	(cached)
ok  	github.com/theapemachine/symm/nomagique/types	(cached)
?   	github.com/theapemachine/symm/nomagique/utils	[no test files]
ok  	github.com/theapemachine/symm/nomagique/vector	(cached)
ok  	github.com/theapemachine/symm/signal	(cached)
ok  	github.com/theapemachine/symm/signal/correlation	(cached)
ok  	github.com/theapemachine/symm/signal/cvd	(cached)
ok  	github.com/theapemachine/symm/signal/depthflow	(cached)
ok  	github.com/theapemachine/symm/signal/derivatives	(cached)
ok  	github.com/theapemachine/symm/signal/hawkes	(cached)
ok  	github.com/theapemachine/symm/signal/leadlag	(cached)
ok  	github.com/theapemachine/symm/signal/liquidity	(cached)
ok  	github.com/theapemachine/symm/signal/morphology	(cached)
ok  	github.com/theapemachine/symm/signal/pumpdump	(cached)
ok  	github.com/theapemachine/symm/signal/sentiment	(cached)
ok  	github.com/theapemachine/symm/signal/toxicity	(cached)
ok  	github.com/theapemachine/symm/store	1.993s
ok  	github.com/theapemachine/symm/strategy	11.069s
ok  	github.com/theapemachine/symm/system	(cached)
ok  	github.com/theapemachine/symm/telemetry	(cached)
?   	github.com/theapemachine/symm/telemetry/generated/telemetry	[no test files]
ok  	github.com/theapemachine/symm/tests/market	(cached)
ok  	github.com/theapemachine/symm/tools/metriclineage	3.088s
ok  	github.com/theapemachine/symm/tools/metricmap	(cached)
ok  	github.com/theapemachine/symm/types	(cached)
ok  	github.com/theapemachine/symm/ui	1.331s
?   	github.com/theapemachine/symm/utils	[no test files]
```

Stderr:

```text
# github.com/theapemachine/symm/broker.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd/hindsight_export.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/kraken/websocket.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/category.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/cognition.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/manifold.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/resonance.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/nomagique/physics/sensorium.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/nomagique/runtime.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/correlation.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/cvd.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/depthflow.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/derivatives.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/hawkes.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/leadlag.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/liquidity.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/morphology.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/pumpdump.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/sentiment.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/toxicity.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/system.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/types.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/ui.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

### benchmarks

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -p 1 -run '^$' -bench 'Benchmark(ModelSelect|ModelRecall|PriorReading|CapitalKnowledgeReading|CapitalKnowledgeIssue|CapitalLearnerAllocate|AccountTeacherObserve|VirtualPortfolioSnapshot|RecordLifecycle)$' -benchmem ./nomagique/learning ./strategy ./cmd
```

```text
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/nomagique/learning
cpu: Apple M4 Max
BenchmarkModelRecall-16    	 4570926	       247.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkModelSelect-16    	 2271409	       557.7 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/theapemachine/symm/nomagique/learning	2.635s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/strategy
cpu: Apple M4 Max
BenchmarkAccountTeacherObserve-16       	11209474	       117.0 ns/op	     249 B/op	       1 allocs/op
BenchmarkCapitalKnowledgeReading-16     	 2217094	       556.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkCapitalKnowledgeIssue-16       	 4437262	       268.4 ns/op	      80 B/op	       1 allocs/op
BenchmarkCapitalLearnerAllocate-16      	   16618	     75859 ns/op	   79394 B/op	     369 allocs/op
BenchmarkVirtualPortfolioSnapshot-16    	 4577228	       254.2 ns/op	     104 B/op	      10 allocs/op
PASS
ok  	github.com/theapemachine/symm/strategy	6.423s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/cmd
cpu: Apple M4 Max
BenchmarkRecordLifecycle-16    	 2721799	       445.1 ns/op	     632 B/op	      12 allocs/op
PASS
ok  	github.com/theapemachine/symm/cmd	1.639s
```

Stderr:

```text
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

### typecheck

```sh
pnpm typecheck
```

```text

> my-tanstack-app@ typecheck /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend
> tsc -p tsconfig.typecheck.json --noEmit

```

### frontend-tests

```sh
pnpm test
```

```text

> my-tanstack-app@ test /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend
> vitest run


 RUN  v4.1.7 /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend


 Test Files  65 passed (65)
      Tests  277 passed (277)
   Start at  02:12:25
   Duration  5.40s (transform 35.92s, setup 0ms, import 45.50s, tests 2.13s, environment 3ms)

```

### frontend-build

```sh
pnpm build
```

```text

> my-tanstack-app@ build /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend
> vite build

vite v8.0.14 building client environment for production...
[2K
transforming...✓ 880 modules transformed.
rendering chunks...
computing gzip size...
dist/client/assets/ws-worker-BrCtasJc.js         1.42 kB
dist/client/assets/graph-DdXj2upj.css            0.55 kB │ gzip:   0.29 kB
dist/client/assets/app-CSwGdDAz.css             70.66 kB │ gzip:  12.50 kB
dist/client/assets/lineage-report-C8bfQqyk.js    0.71 kB │ gzip:   0.43 kB
dist/client/assets/xray-layers-C5zDssnF.js       0.71 kB │ gzip:   0.46 kB
dist/client/assets/sparkline-B4BPPxlc.js         0.78 kB │ gzip:   0.53 kB
dist/client/assets/chip-DBYUM0eC.js              0.85 kB │ gzip:   0.45 kB
dist/client/assets/ui-BniqsLrg.js                0.94 kB │ gzip:   0.40 kB
dist/client/assets/canvas-DT5EcOgS.js            1.03 kB │ gzip:   0.50 kB
dist/client/assets/canvas-DSRIxc1H.js            1.17 kB │ gzip:   0.66 kB
dist/client/assets/section-C29KLXhy.js           1.33 kB │ gzip:   0.64 kB
dist/client/assets/stat-BxAf8cIT.js              1.62 kB │ gzip:   0.69 kB
dist/client/assets/tabs-XINL06zV.js              2.63 kB │ gzip:   1.08 kB
dist/client/assets/meter-D8hFuyvQ.js             3.06 kB │ gzip:   1.05 kB
dist/client/assets/journal-DDO8SYuO.js           8.16 kB │ gzip:   2.50 kB
dist/client/assets/lineage-Cmx2-HNw.js           9.12 kB │ gzip:   3.09 kB
dist/client/assets/allocation-DTY618u9.js        9.27 kB │ gzip:   2.36 kB
dist/client/assets/regulator-BBOnTmQw.js         9.31 kB │ gzip:   2.74 kB
dist/client/assets/influence-C6rVeW3q.js        11.16 kB │ gzip:   4.13 kB
dist/client/assets/signals-BRqZHuR0.js          11.22 kB │ gzip:   3.46 kB
dist/client/assets/charts-Hi_KWF-a.js           11.26 kB │ gzip:   3.24 kB
dist/client/assets/kernel-list-4l2kfcaN.js      11.41 kB │ gzip:   4.31 kB
dist/client/assets/cortex-B6s_EJvU.js           16.09 kB │ gzip:   5.36 kB
dist/client/assets/xray-DRLwMI5A.js             20.86 kB │ gzip:   6.83 kB
dist/client/assets/diagnostics-blZPi0RZ.js      29.95 kB │ gzip:   9.43 kB
dist/client/assets/routes-DKi7y--l.js           31.10 kB │ gzip:   9.80 kB
dist/client/assets/learning-D_bR2LDp.js         40.33 kB │ gzip:  11.46 kB
dist/client/assets/fluid-DWfIui-Y.js            50.84 kB │ gzip:  15.78 kB
dist/client/assets/hindsight-DxgY-XNd.js        98.29 kB │ gzip:  26.66 kB
dist/client/assets/graph-ekEFpfx4.js           690.85 kB │ gzip: 177.91 kB
dist/client/assets/index-Ca31Y-TJ.js           892.89 kB │ gzip: 217.31 kB

✓ built in 927ms
vite v8.0.14 building ssr environment for production...
[2K
transforming...✓ 368 modules transformed.
rendering chunks...
computing gzip size...
dist/server/assets/__23tanstack-start-plugin-adapters-BzCA6dXo.js    0.18 kB │ gzip:  0.13 kB
dist/server/assets/utils-BK-ZNMQQ.js                                 0.19 kB │ gzip:  0.15 kB
dist/server/assets/start-B4GG2V6N.js                                 0.28 kB │ gzip:  0.20 kB
dist/server/assets/toolbar-CsJWPrPy.js                               1.10 kB │ gzip:  0.51 kB
dist/server/assets/chip-DyymHjF2.js                                  1.23 kB │ gzip:  0.62 kB
dist/server/assets/sparkline-KzmYnco-.js                             1.43 kB │ gzip:  0.71 kB
dist/server/assets/lineage-report-C5MeIjrX.js                        1.47 kB │ gzip:  0.68 kB
dist/server/assets/panel-CPFwpbYx.js                                 1.49 kB │ gzip:  0.64 kB
dist/server/assets/xray-layers-BXKE1M7y.js                           1.52 kB │ gzip:  0.70 kB
dist/server/assets/terminal-Wr2Py6vs.js                              1.69 kB │ gzip:  0.51 kB
dist/server/assets/input-ClFkh_Go.js                                 1.87 kB │ gzip:  0.82 kB
dist/server/assets/named-number-CRkesQVk.js                          1.94 kB │ gzip:  0.67 kB
dist/server/assets/named-string-BFEqYZQ2.js                          2.07 kB │ gzip:  0.67 kB
dist/server/assets/section-Dmfllipw.js                               2.07 kB │ gzip:  0.83 kB
dist/server/assets/list-BmrpVT0B.js                                  2.15 kB │ gzip:  0.91 kB
dist/server/assets/canvas-C3D8UK97.js                                2.36 kB │ gzip:  0.94 kB
dist/server/assets/canvas-5YObKxHE.js                                2.40 kB │ gzip:  0.93 kB
dist/server/assets/stat-ZKkJ7G2T.js                                  2.55 kB │ gzip:  0.94 kB
dist/server/assets/button-BH5tPtHt.js                                2.84 kB │ gzip:  1.03 kB
dist/server/assets/badge-CRFNqu9M.js                                 3.02 kB │ gzip:  1.09 kB
dist/server/assets/tabs-D3R7wFq6.js                                  3.57 kB │ gzip:  1.32 kB
dist/server/assets/subsystem-CfRBR717.js                             4.50 kB │ gzip:  1.00 kB
dist/server/assets/meter-BVNlFotO.js                                 4.52 kB │ gzip:  1.36 kB
dist/server/assets/flex-7F99uGv1.js                                  4.64 kB │ gzip:  1.38 kB
dist/server/assets/_tanstack-start-manifest_v-En_On0QG.js            4.76 kB │ gzip:  0.99 kB
dist/server/assets/icon-zR0uas9A.js                                  5.03 kB │ gzip:  1.26 kB
dist/server/assets/typography-DEbp9a0c.js                            5.61 kB │ gzip:  1.63 kB
dist/server/assets/app-DacQGu1m.js                                   6.97 kB │ gzip:  1.99 kB
dist/server/assets/envelope-measurement-metric-CLZvuaDi.js           7.12 kB │ gzip:  1.35 kB
dist/server/assets/router-otLYi7Cr.js                                7.93 kB │ gzip:  2.03 kB
dist/server/assets/position-BXpylJmV.js                             11.48 kB │ gzip:  1.92 kB
dist/server/assets/envelope-cognition-class-B5nN0UuT.js             11.67 kB │ gzip:  1.75 kB
dist/server/assets/regulator-D0Rj6I-I.js                            13.71 kB │ gzip:  3.43 kB
dist/server/assets/ui-C2SFcJym.js                                   14.48 kB │ gzip:  3.80 kB
dist/server/assets/allocation-_F8nnGOg.js                           14.51 kB │ gzip:  2.99 kB
dist/server/assets/journal-PxhZjhQR.js                              15.63 kB │ gzip:  3.57 kB
dist/server/assets/kernel-list-R_ndp9qB.js                          15.81 kB │ gzip:  5.15 kB
dist/server/assets/lineage-BnyIlGZf.js                              16.23 kB │ gzip:  4.33 kB
dist/server/assets/signals-ByUgdSIj.js                              18.64 kB │ gzip:  4.52 kB
dist/server/assets/charts-BnkOCqU0.js                               19.25 kB │ gzip:  4.49 kB
dist/server/assets/influence-DviauXkc.js                            21.28 kB │ gzip:  5.87 kB
dist/server/assets/decision-CE-_OC_m.js                             27.80 kB │ gzip:  4.11 kB
dist/server/assets/cortex-CJf1M-kr.js                               30.10 kB │ gzip:  7.61 kB
dist/server/assets/xray-Bj6KGjDh.js                                 37.33 kB │ gzip:  9.41 kB
dist/server/assets/routes-BZkY2QG5.js                               49.18 kB │ gzip: 12.51 kB
dist/server/assets/graph-frame-Ddjgtjya.js                          52.24 kB │ gzip:  6.08 kB
dist/server/assets/diagnostics-DlkDdlbQ.js                          54.95 kB │ gzip: 13.77 kB
dist/server/assets/learning-BiIEIphX.js                             68.57 kB │ gzip: 15.19 kB
dist/server/assets/hindsight-CKrYMLWt.js                           160.85 kB │ gzip: 35.31 kB
dist/server/server.js                                              163.52 kB │ gzip: 41.23 kB
dist/server/assets/graph-Dvb8GCKp.js                               171.56 kB │ gzip: 32.90 kB
dist/server/assets/envelope-state-Bom6W4xQ.js                      235.14 kB │ gzip: 27.36 kB
dist/server/assets/fluid-CcwE7R13.js                               318.33 kB │ gzip: 48.10 kB

✓ built in 698ms
```

Stderr:

```text
[plugin builtin:vite-reporter] 
(!) Some chunks are larger than 500 kB after minification. Consider:
- Using dynamic import() to code-split the application
- Use build.rolldownOptions.output.codeSplitting to improve chunking: https://rolldown.rs/reference/OutputOptions.codeSplitting
- Adjust chunk size limit for this warning via build.chunkSizeWarningLimit.
```

### build

```sh
GOFLAGS=-ldflags=-checklinkname=0 go build -o /tmp/symm-capital-refinement/symm .
```

```text
```

Stderr:

```text
# github.com/theapemachine/symm
ld: warning: ignoring duplicate libraries: '-lc++'
```

### vet

```sh
GOFLAGS=-ldflags=-checklinkname=0 go vet -p 1 ./...
```

```text
```

Vet and the Go build emitted no stdout. Go linking retains the duplicate -lc++ warning. Frontend build reports its existing large chunk warning. No dependency, permission or service prevented verification. A short live run does not establish profitable allocation or a venue/IP-rotation endurance guarantee.

## Full streaming Agent benchmark

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -p 1 -run '^$' -bench '^BenchmarkAgentStep$' -benchmem ./strategy
```

```text
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/strategy
cpu: Apple M4 Max
BenchmarkAgentStep-16    	   15759	     76908 ns/op	   47579 B/op	    2070 allocs/op
PASS
ok  	github.com/theapemachine/symm/strategy	1.599s
```

Stderr:

```text
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
```
