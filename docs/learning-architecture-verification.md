# SYMM learning verification

Verification completed on 2026-09-06, macOS arm64, Apple M4 Max. Each automated command below exited 0. Standard output is copied literally; stderr is stored separately. Cached Go results are identified by Go itself. The Makefile supplies the required qpool linker flag and serial package execution.

## Runtime evidence

The paper backend ran against authenticated live Kraken feeds from 00:36:44 to 00:41:44 Europe/Rome, using an isolated data path. The existing main database was locked by another running process; that process and database were left alone. The isolated configuration copied `cmd/cfg/config.yml` and changed only `system.data_path`.

```sh
go run -ldflags=-checklinkname=0 main.go --config /tmp/symm-learning-verification/config.yml
```

The server listened on `127.0.0.1:8765`. It recovered 4,095 complete local experiences and 56 capital experiences before starting with cold Skill. The last successful HTTP snapshot contains 1,903,515 local resolutions, 428 spot symbols, 664 quantities, 66 actual account allocations, and 25 exploration allocations. Browser inspection verified the capital panel, separate increase gates, rational cash displays, hierarchical readings, candidate selection, and a later local outcome appearing separately in Forward Review. This run did not earn execution authority and does not prove profitable trading or real venue order placement. Order submission, scale fills, cancellation, liquidation, cash refusal, and Realization routing are covered by deterministic lifecycle tests.

The first live run exposed dormant-prior squared-weight underflow. A regression reproduced the NaN before the representation fix. The final live run crossed that earlier failure point with finite journal and UI values. The captured last snapshot and one complete candidate journey are included below. The final virtual reservation display and warmup index refinements were verified by the final automated checks after the live run.

- [Final live stdout](learning-verification/final-live.stdout)
- [Final live stderr](learning-verification/final-live.stderr) — linker warning and the intentional interrupt used to stop the isolated run.
- [Final live snapshot](learning-verification/final-live-last.json)
- [Prospective candidate and later local resolution](learning-verification/final-resolved-candidate.json)
- [Snapshot assertions](learning-verification/check_live.py)
- [Failing numerical regression before the fix](learning-verification/prior-regression-before.stdout)

The isolated retained tape remains at `/tmp/symm-learning-verification/data/events.sqlite` (about 21 GiB); no production data was removed. Browser screenshots were inspected in the task; the browser connector did not permit saving them to this workspace.

## Retained-data query

On that captured database, the query plan changed from a full resolved-event scan to the new kind index. Index construction was a one-time migration. The following output is literal:

```text
Before: [(6, 0, 216, 'SCAN resolved'), (11, 0, 60, 'SEARCH issued USING INDEX learning_events_identity (run_id=? AND <expr>=? AND <expr>=?)')]
One-time index construction seconds: 19.470011
After: [(7, 0, 61, 'SEARCH resolved USING INDEX learning_events_kind (<expr>=?)'), (13, 0, 60, 'SEARCH issued USING INDEX learning_events_identity (run_id=? AND <expr>=? AND <expr>=?)')]
Complete portfolio pairs: 147
Indexed retained-pair query seconds: 0.001618
```

## Automated commands and literal stdout

### test-go

```sh
make test-go
```

```text
go test -ldflags=-checklinkname=0 -p 1 -timeout 30m ./...
?   	github.com/theapemachine/symm	[no test files]
ok  	github.com/theapemachine/symm/broker	4.484s
ok  	github.com/theapemachine/symm/cmd	0.516s
ok  	github.com/theapemachine/symm/cmd/hindsight_export	0.332s
?   	github.com/theapemachine/symm/cmd/hindsight_probe	[no test files]
ok  	github.com/theapemachine/symm/hindsight	0.254s
ok  	github.com/theapemachine/symm/kraken	0.392s
ok  	github.com/theapemachine/symm/kraken/websocket	1.127s
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
ok  	github.com/theapemachine/symm/nomagique/learning	0.308s
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
ok  	github.com/theapemachine/symm/store	0.447s
ok  	github.com/theapemachine/symm/strategy	0.972s
ok  	github.com/theapemachine/symm/system	(cached)
ok  	github.com/theapemachine/symm/telemetry	(cached)
?   	github.com/theapemachine/symm/telemetry/generated/telemetry	[no test files]
ok  	github.com/theapemachine/symm/tests/market	(cached)
ok  	github.com/theapemachine/symm/tools/metriclineage	1.411s
ok  	github.com/theapemachine/symm/tools/metricmap	(cached)
ok  	github.com/theapemachine/symm/types	0.283s
ok  	github.com/theapemachine/symm/ui	0.310s
?   	github.com/theapemachine/symm/utils	[no test files]
```

[Unmodified stderr](learning-verification/test-go.stderr).

### test-race

```sh
make test-race
```

```text
go test -ldflags=-checklinkname=0 -p 1 -timeout 30m -race ./...
?   	github.com/theapemachine/symm	[no test files]
ok  	github.com/theapemachine/symm/broker	6.623s
ok  	github.com/theapemachine/symm/cmd	1.504s
ok  	github.com/theapemachine/symm/cmd/hindsight_export	1.321s
?   	github.com/theapemachine/symm/cmd/hindsight_probe	[no test files]
ok  	github.com/theapemachine/symm/hindsight	1.391s
ok  	github.com/theapemachine/symm/kraken	1.284s
ok  	github.com/theapemachine/symm/kraken/websocket	3.247s
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
ok  	github.com/theapemachine/symm/nomagique/learning	1.740s
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
ok  	github.com/theapemachine/symm/store	1.925s
ok  	github.com/theapemachine/symm/strategy	9.249s
ok  	github.com/theapemachine/symm/system	(cached)
ok  	github.com/theapemachine/symm/telemetry	(cached)
?   	github.com/theapemachine/symm/telemetry/generated/telemetry	[no test files]
ok  	github.com/theapemachine/symm/tests/market	(cached)
ok  	github.com/theapemachine/symm/tools/metriclineage	2.855s
ok  	github.com/theapemachine/symm/tools/metricmap	(cached)
ok  	github.com/theapemachine/symm/types	1.418s
ok  	github.com/theapemachine/symm/ui	1.356s
?   	github.com/theapemachine/symm/utils	[no test files]
```

[Unmodified stderr](learning-verification/test-race.stderr).

### vet

```sh
GOFLAGS=-ldflags=-checklinkname=0 go vet -p 1 ./...
```

Stdout was empty.

[Unmodified stderr](learning-verification/vet.stderr).

### bench

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -p 1 -run '^$' -bench 'Benchmark(AgentStep|KnowledgeSelect|CapitalLearnerAllocate|VirtualPortfolioSnapshot|AccountTeacherObserve|CandidateBookViable|AccountFundsObserve|RecordLifecycle|WriterWriteLearning|SQLiteLearningExperiences|LotIncreaseApply|ModelResolve|ModelRecall|ModelSelect|PriorObserve|FundingLedgerObserve|ExtendedBalanceAvailable)$' -benchmem ./nomagique/learning ./strategy ./cmd ./broker ./store ./kraken ./kraken/websocket
```

```text
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/nomagique/learning
cpu: Apple M4 Max
BenchmarkModelResolve-16    	 4601516	       260.7 ns/op	      32 B/op	       1 allocs/op
BenchmarkModelRecall-16     	 6707810	       178.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkPriorObserve-16    	89583439	        13.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkModelSelect-16     	 2751675	       435.5 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/theapemachine/symm/nomagique/learning	4.929s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/strategy
cpu: Apple M4 Max
BenchmarkAccountTeacherObserve-16       	12029984	        94.05 ns/op	     249 B/op	       1 allocs/op
BenchmarkAgentStep-16                   	   23932	     50465 ns/op	   47248 B/op	    2068 allocs/op
BenchmarkCandidateBookViable-16         	21121646	        56.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkCapitalLearnerAllocate-16      	   34320	     34153 ns/op	   62950 B/op	     369 allocs/op
BenchmarkKnowledgeSelect-16             	 2037913	       588.6 ns/op	      24 B/op	       1 allocs/op
BenchmarkVirtualPortfolioSnapshot-16    	 5398372	       223.2 ns/op	     104 B/op	      10 allocs/op
PASS
ok  	github.com/theapemachine/symm/strategy	7.274s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/cmd
cpu: Apple M4 Max
BenchmarkAccountFundsObserve-16    	134874182	         8.861 ns/op	       0 B/op	       0 allocs/op
BenchmarkRecordLifecycle-16        	 5841222	       204.5 ns/op	     456 B/op	       4 allocs/op
PASS
ok  	github.com/theapemachine/symm/cmd	2.670s
{"date":"2026-09-06 00:44:28","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":20,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 00:44:28","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":20,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 00:44:28","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":20,"message":"websocket: initializing book manager"}
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/broker
cpu: Apple M4 Max
BenchmarkLotIncreaseApply-16    	  588505	      1933 ns/op	    3682 B/op	     146 allocs/op
PASS
ok  	github.com/theapemachine/symm/broker	1.322s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/store
cpu: Apple M4 Max
BenchmarkWriterWriteLearning-16          	   52935	     22974 ns/op	    2633 B/op	      20 allocs/op
BenchmarkSQLiteLearningExperiences-16    	   15922	     74537 ns/op	   92985 B/op	     279 allocs/op
PASS
ok  	github.com/theapemachine/symm/store	2.742s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/kraken
cpu: Apple M4 Max
BenchmarkExtendedBalanceAvailable-16    	 8199080	       142.8 ns/op	     216 B/op	       9 allocs/op
PASS
ok  	github.com/theapemachine/symm/kraken	1.321s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/kraken/websocket
cpu: Apple M4 Max
BenchmarkFundingLedgerObserve-16    	 1888610	       639.5 ns/op	     608 B/op	      16 allocs/op
PASS
ok  	github.com/theapemachine/symm/kraken/websocket	1.529s
```

[Unmodified stderr](learning-verification/bench.stderr).

### frontend-typecheck

```sh
pnpm --dir frontend typecheck
```

```text

> my-tanstack-app@ typecheck /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend
> tsc -p tsconfig.typecheck.json --noEmit

```

[Unmodified stderr](learning-verification/frontend-typecheck.stderr).

### frontend-test

```sh
pnpm --dir frontend test
```

```text

> my-tanstack-app@ test /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend
> vitest run


 RUN  v4.1.7 /Users/theapemachine/go/src/github.com/theapemachine/symm/frontend


 Test Files  65 passed (65)
      Tests  273 passed (273)
   Start at  00:40:36
   Duration  4.67s (transform 31.12s, setup 0ms, import 41.13s, tests 1.91s, environment 6ms)

```

[Unmodified stderr](learning-verification/frontend-test.stderr).

### frontend-build

```sh
pnpm --dir frontend build
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
dist/client/assets/app-BxyH6NiV.css             70.58 kB │ gzip:  12.48 kB
dist/client/assets/lineage-report-C7N36e6f.js    0.71 kB │ gzip:   0.43 kB
dist/client/assets/xray-layers-nXLwt2gA.js       0.71 kB │ gzip:   0.45 kB
dist/client/assets/sparkline-C0pcEfuh.js         0.78 kB │ gzip:   0.53 kB
dist/client/assets/chip-wn7T_ZZm.js              0.85 kB │ gzip:   0.45 kB
dist/client/assets/ui-jOzQSh6o.js                0.94 kB │ gzip:   0.39 kB
dist/client/assets/canvas-fccW5IK3.js            1.03 kB │ gzip:   0.50 kB
dist/client/assets/canvas-DSRIxc1H.js            1.17 kB │ gzip:   0.66 kB
dist/client/assets/section-Bi_Smgn-.js           1.33 kB │ gzip:   0.64 kB
dist/client/assets/stat-2681BjZd.js              1.62 kB │ gzip:   0.69 kB
dist/client/assets/tabs-Ngrl2nDs.js              2.63 kB │ gzip:   1.08 kB
dist/client/assets/meter-BuCofChb.js             3.06 kB │ gzip:   1.05 kB
dist/client/assets/journal-D8JOZaO9.js           8.16 kB │ gzip:   2.49 kB
dist/client/assets/lineage-CZgHX2fq.js           9.12 kB │ gzip:   3.09 kB
dist/client/assets/allocation-DO1uCFB3.js        9.27 kB │ gzip:   2.36 kB
dist/client/assets/regulator-BxNtsTVg.js         9.31 kB │ gzip:   2.74 kB
dist/client/assets/influence-CQY06V3d.js        11.16 kB │ gzip:   4.13 kB
dist/client/assets/signals-OeV4nBLb.js          11.22 kB │ gzip:   3.46 kB
dist/client/assets/charts-DmhB35PI.js           11.26 kB │ gzip:   3.24 kB
dist/client/assets/kernel-list-DbKzsuVa.js      11.41 kB │ gzip:   4.31 kB
dist/client/assets/cortex-BSnB5Bz9.js           16.09 kB │ gzip:   5.36 kB
dist/client/assets/xray-71EPgtpX.js             20.86 kB │ gzip:   6.83 kB
dist/client/assets/diagnostics-CFPjv0xS.js      29.95 kB │ gzip:   9.42 kB
dist/client/assets/routes-5oEvric1.js           31.10 kB │ gzip:   9.80 kB
dist/client/assets/learning-DXwTMnlW.js         39.18 kB │ gzip:  11.18 kB
dist/client/assets/fluid-DzvM2cKT.js            50.84 kB │ gzip:  15.78 kB
dist/client/assets/hindsight-B_nWQQlE.js        98.29 kB │ gzip:  26.66 kB
dist/client/assets/graph-BdjN5BPw.js           690.85 kB │ gzip: 177.91 kB
dist/client/assets/index-Dz300UI2.js           892.58 kB │ gzip: 217.22 kB

✓ built in 1.23s
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
dist/server/assets/_tanstack-start-manifest_v-DJvkwiuL.js            4.76 kB │ gzip:  1.00 kB
dist/server/assets/icon-zR0uas9A.js                                  5.03 kB │ gzip:  1.26 kB
dist/server/assets/typography-DEbp9a0c.js                            5.61 kB │ gzip:  1.63 kB
dist/server/assets/app-DacQGu1m.js                                   6.97 kB │ gzip:  1.99 kB
dist/server/assets/envelope-measurement-metric-CLZvuaDi.js           7.12 kB │ gzip:  1.35 kB
dist/server/assets/router-BcQW1cC8.js                                7.93 kB │ gzip:  2.03 kB
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
dist/server/assets/learning-DCeKyg46.js                             66.72 kB │ gzip: 14.88 kB
dist/server/assets/hindsight-CKrYMLWt.js                           160.85 kB │ gzip: 35.31 kB
dist/server/server.js                                              163.52 kB │ gzip: 41.23 kB
dist/server/assets/graph-Dvb8GCKp.js                               171.56 kB │ gzip: 32.90 kB
dist/server/assets/envelope-state-Bom6W4xQ.js                      235.14 kB │ gzip: 27.36 kB
dist/server/assets/fluid-CcwE7R13.js                               318.33 kB │ gzip: 48.10 kB

✓ built in 876ms
```

[Unmodified stderr](learning-verification/frontend-build.stderr).

### live-check

```sh
python3 /tmp/symm-learning-verification/check_live.py
```

```text
Snapshot source: live /learning HTTP response
Snapshot at: 2026-09-06T00:41:44.096057+02:00
Spot policy symbols: 428
Numerical quantities: 664
Local decisions: 1946490
Local resolutions: 1903515
Actual allocation resolutions: 66
Virtual allocation resolutions: 25
Effective mode: learning
Candidate causal identity: 03c081c2-7c5d-4bef-8443-71a408b022a7
Candidate event kinds: issued, candidate, candidate_status, resolved
PASS: finite JSON, spot-only policy, cold authority, complete funding, separate prospective and later labels
```

## Warnings and verification boundaries

Go linking emits the existing duplicate `-lc++` warning. The frontend build emits the Node `module.register()` deprecation and a large chunk warning; both builds completed. Vet produced no diagnostics. The old live-WebSocket test filenames mentioned by the memory skill no longer exist in this checkout; actual browser and HTTP inspection supplied live delivery evidence. The isolated paper backend was stopped after inspection; no profiler was started.

No real-account deposit/withdrawal experiment or live venue order was forced. Non-quote external funding remains explicitly unavailable without historical valuation; cold Skill remains the actual increase gate. See the implementation report for the complete economic and causal contracts.
