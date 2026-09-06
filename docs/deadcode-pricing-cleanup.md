# Dead-code and pricing cleanup

Removed 101 production-unreachable functions outside nomagique, their orphaned types and retired-only tests. Removed the additional unused liquidity test helper found by the test-rooted scan. Production scanning retains 14 shared tests/market fixture functions; the test-rooted scan reports zero unreachable functions outside nomagique. Existing nomagique APIs and the earlier manifold work are unchanged by this cleanup.

## Pricing ownership

`broker/price.go` owns taker percentage conversion, exact fee application, lot flooring, affordable quantity, executable book sweeps, entry economics, liquidation surface math, notional/VWAP, partial cost/fee allocation and return arithmetic. `broker/quote.go` owns live market-data lookup and quote validation. Strategy wallets compose the same Pricing owner and retain their account balances and learning policy. `broker/depth_ladder.go` owns the existing retained decision-depth constraint; there is no second strategy sweep or fee implementation.

Fixed known-depth fallback to ticker, one-sided resident-book fallback, loss of tiny notionals/fees from decimal receiver precision, fees recomputed from rounded VWAP, invalid trade directions returning unchanged amounts, and SDK nearest-lot rounding exceeding available cash. Realized fills, lot increases, recovery and execution feedback use the same arithmetic. Benchmark fixtures now supply authoritative marks and actual book liquidity.

Protocol unit conversion in the Kraken paper adapter, authoritative funding-ledger additions/subtractions, descriptive signal geometry and empirical slippage statistics remain at their own boundaries; these do not define executable trade prices or estimated trading fees. No database was discarded and no live orders or trading session were started. Verification proves automated behavior and race checks, not a live-market soak. Exact rational arithmetic remains allocating; measured costs are below.

## Changed files

- `broker/depth_ladder.go`
- `broker/depth_ladder_test.go`
- `broker/entry_economics.go`
- `broker/entry_economics_test.go`
- `broker/lot_increase.go`
- `broker/position.go`
- `broker/price.go`
- `broker/price_fee.go`
- `broker/price_test.go`
- `broker/quote.go`
- `broker/quote_test.go`
- `broker/recovery.go`
- `cmd/cfg/config.yml`
- `cmd/execution_feedback.go`
- `cmd/hindsight_nodes.go`
- `cmd/learning_desk.go`
- `docs/deadcode-pricing-cleanup.md`
- `hindsight/replay.go` (deleted)
- `hindsight/replay_test.go` (deleted)
- `hindsight/tape.go` (deleted)
- `hindsight/tape_test.go` (deleted)
- `hindsight/verify.go` (deleted)
- `hindsight/wire.go`
- `hindsight/wire_test.go`
- `kraken/book.go`
- `kraken/data.go` (deleted)
- `kraken/execution.go`
- `kraken/instrument.go`
- `kraken/level3.go`
- `kraken/ohlc.go`
- `kraken/order.go`
- `kraken/position.go` (deleted)
- `kraken/reqid.go` (deleted)
- `kraken/ticker.go`
- `kraken/trade.go`
- `kraken/trade_balance.go`
- `kraken/trades_history.go`
- `kraken/websocket/book.go`
- `kraken/websocket/book_test.go`
- `kraken/websocket/live.go`
- `kraken/websocket/live_test.go`
- `logic/category/solver.go`
- `logic/category/solver_test.go`
- `logic/cognition/solver.go`
- `logic/cognition/solver_test.go`
- `logic/cognition/wire.go` (deleted)
- `logic/manifold/phase_scan.go` (deleted)
- `signal/liquidity/ticker_test.go`
- `signal/morphology/book.go`
- `signal/morphology/book_test.go`
- `signal/semantics.go`
- `signal/semantics_test.go`
- `strategy/candidate_price.go`
- `strategy/candidates.go`
- `strategy/candidates_test.go`
- `strategy/execution.go`
- `strategy/lane.go`
- `strategy/local.go`
- `strategy/portfolio.go`
- `strategy/virtual.go`
- `strategy/virtual_test.go`
- `system/config.go`
- `system/config_test.go`
- `system/regulator.go`
- `system/regulator_test.go`
- `telemetry/encode.go` (deleted)
- `telemetry/encode_test.go` (deleted)
- `types/opportunity.go`
- `types/passage.go` (deleted)
- `types/passage_test.go` (deleted)

## Verification (literal final stdout and stderr)

```sh
GOFLAGS='-ldflags=-checklinkname=0' go test -p 1 -timeout 30m ./...
```

stdout:

```text
?   	github.com/theapemachine/symm	[no test files]
ok  	github.com/theapemachine/symm/broker	5.952s
ok  	github.com/theapemachine/symm/cmd	(cached)
ok  	github.com/theapemachine/symm/cmd/hindsight_export	(cached)
?   	github.com/theapemachine/symm/cmd/hindsight_probe	[no test files]
ok  	github.com/theapemachine/symm/hindsight	(cached)
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
ok  	github.com/theapemachine/symm/nomagique/learning	(cached)
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
ok  	github.com/theapemachine/symm/signal/liquidity	0.409s
ok  	github.com/theapemachine/symm/signal/morphology	(cached)
ok  	github.com/theapemachine/symm/signal/pumpdump	(cached)
ok  	github.com/theapemachine/symm/signal/sentiment	(cached)
ok  	github.com/theapemachine/symm/signal/toxicity	(cached)
ok  	github.com/theapemachine/symm/store	(cached)
ok  	github.com/theapemachine/symm/strategy	(cached)
ok  	github.com/theapemachine/symm/system	(cached)
?   	github.com/theapemachine/symm/telemetry/generated/telemetry	[no test files]
ok  	github.com/theapemachine/symm/tests/market	(cached)
ok  	github.com/theapemachine/symm/tools/manifoldexperiment	(cached)
ok  	github.com/theapemachine/symm/tools/metriclineage	(cached)
ok  	github.com/theapemachine/symm/tools/metricmap	(cached)
ok  	github.com/theapemachine/symm/types	(cached)
ok  	github.com/theapemachine/symm/ui	(cached)
?   	github.com/theapemachine/symm/utils	[no test files]
```

stderr:

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
# github.com/theapemachine/symm/tools/manifoldexperiment.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/types.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/ui.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

```sh
GOFLAGS='-ldflags=-checklinkname=0' go test -race -p 1 -timeout 30m ./broker ./strategy ./hindsight ./kraken/... ./signal/... ./logic/category ./logic/cognition ./logic/manifold ./system ./types ./cmd
```

stdout:

```text
ok  	github.com/theapemachine/symm/broker	8.665s
ok  	github.com/theapemachine/symm/strategy	11.917s
ok  	github.com/theapemachine/symm/hindsight	(cached)
ok  	github.com/theapemachine/symm/kraken	(cached)
ok  	github.com/theapemachine/symm/kraken/websocket	(cached)
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
ok  	github.com/theapemachine/symm/logic/category	(cached)
ok  	github.com/theapemachine/symm/logic/cognition	(cached)
ok  	github.com/theapemachine/symm/logic/manifold	(cached)
ok  	github.com/theapemachine/symm/system	(cached)
ok  	github.com/theapemachine/symm/types	(cached)
ok  	github.com/theapemachine/symm/cmd	1.551s
```

stderr:

```text
# github.com/theapemachine/symm/broker.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/kraken/websocket.test
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
# github.com/theapemachine/symm/logic/category.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/cognition.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/manifold.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/system.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/types.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/cmd.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

```sh
GOFLAGS='-ldflags=-checklinkname=0' go test -p 1 -run '^$' -bench 'Benchmark(Pricing|Prorate|DepthLadder|Price|VirtualWallet|Step|SolverStepMeasurement)' -benchmem ./broker ./strategy ./signal/morphology ./logic/category
```

stdout:

```text
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/broker
cpu: Apple M4 Max
BenchmarkDepthLadderSurviving-16    	 1000000	      1110 ns/op	    1633 B/op	      34 allocs/op
{"date":"2026-09-06 05:39:53","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":51,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:39:53","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":51,"message":"websocket: initializing book manager"}
BenchmarkPriceEntryCost-16          	   54861	     22078 ns/op	   22973 B/op	     850 allocs/op
{"date":"2026-09-06 05:39:54","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":53,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:39:54","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":53,"message":"websocket: initializing book manager"}
BenchmarkPriceFee-16                	18890881	        67.72 ns/op	      80 B/op	       1 allocs/op
{"date":"2026-09-06 05:39:55","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":36,"message":"websocket: initializing book manager"}
BenchmarkPriceResolveFee-16         	 6259780	       211.8 ns/op	     320 B/op	       1 allocs/op
{"date":"2026-09-06 05:39:57","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":38,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:39:57","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":38,"message":"websocket: initializing book manager"}
BenchmarkPriceWithFee-16            	  369417	      2916 ns/op	    3178 B/op	     125 allocs/op
{"date":"2026-09-06 05:39:58","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":40,"message":"websocket: initializing book manager"}
BenchmarkPricingSweep-16            	  257908	      4719 ns/op	    4315 B/op	     185 allocs/op
BenchmarkProrate-16                 	  524580	      2328 ns/op	    2818 B/op	     105 allocs/op
{"date":"2026-09-06 05:40:00","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":55,"message":"websocket: initializing book manager"}
BenchmarkPricingSurface-16          	  121138	      8857 ns/op	    7535 B/op	     311 allocs/op
{"date":"2026-09-06 05:40:01","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":83,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:01","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":83,"message":"websocket: initializing book manager"}
BenchmarkPriceUpdate-16             	15948291	        76.31 ns/op	      64 B/op	       2 allocs/op
{"date":"2026-09-06 05:40:03","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":26,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:03","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":26,"message":"websocket: initializing book manager"}
BenchmarkPriceMark-16               	  353551	      3527 ns/op	    3275 B/op	     128 allocs/op
{"date":"2026-09-06 05:40:04","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":99,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:04","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":99,"message":"websocket: initializing book manager"}
BenchmarkPricePnL-16                	  129192	     10775 ns/op	   11370 B/op	     434 allocs/op
{"date":"2026-09-06 05:40:05","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":115,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:05","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":115,"message":"websocket: initializing book manager"}
BenchmarkPriceExitValue-16          	  230808	      4992 ns/op	    5252 B/op	     203 allocs/op
{"date":"2026-09-06 05:40:06","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":57,"message":"websocket: initializing book manager"}
BenchmarkPriceWithFriction-16       	   74600	     15695 ns/op	   16368 B/op	     643 allocs/op
{"date":"2026-09-06 05:40:08","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":117,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:08","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":117,"message":"websocket: initializing book manager"}
BenchmarkPriceTick-16               	35536689	        35.17 ns/op	       0 B/op	       0 allocs/op
{"date":"2026-09-06 05:40:09","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":119,"message":"websocket: initializing book manager"}
{"date":"2026-09-06 05:40:09","level":"info","caller":"websocket/book.go:39","callerfunc":"websocket.NewBook","goid":119,"message":"websocket: initializing book manager"}
BenchmarkPriceQuantity-16           	  222616	      6288 ns/op	    6117 B/op	     234 allocs/op
PASS
ok  	github.com/theapemachine/symm/broker	18.593s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/strategy
cpu: Apple M4 Max
BenchmarkVirtualWalletActions/2-16  	  128229	     10917 ns/op	    9520 B/op	     396 allocs/op
BenchmarkVirtualWalletActions/512-16         	  118473	     13769 ns/op	    9522 B/op	     396 allocs/op
PASS
ok  	github.com/theapemachine/symm/strategy	3.307s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/signal/morphology
cpu: Apple M4 Max
BenchmarkStep-16    	   46840	     26808 ns/op	   27683 B/op	    1142 allocs/op
PASS
ok  	github.com/theapemachine/symm/signal/morphology	1.520s
goos: darwin
goarch: arm64
pkg: github.com/theapemachine/symm/logic/category
cpu: Apple M4 Max
BenchmarkSolverStepMeasurement-16    	   40591	     27990 ns/op	   11624 B/op	      16 allocs/op
PASS
ok  	github.com/theapemachine/symm/logic/category	1.426s
```

stderr:

```text
# github.com/theapemachine/symm/broker.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/strategy.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/signal/morphology.test
ld: warning: ignoring duplicate libraries: '-lc++'
# github.com/theapemachine/symm/logic/category.test
ld: warning: ignoring duplicate libraries: '-lc++'
```

## Dead-code verification

```sh
deadcode -json ./... > /private/tmp/symm-cleanup-final-dead.json
deadcode -test -json ./... > /private/tmp/symm-cleanup-final-dead-tests.json
```

Both commands completed successfully. Excluding nomagique, the first JSON contains the 14 shared market-fixture functions; the second contains none. Generated schemas were retained for their actual consumers. The original dead-code baseline contained 115 non-nomagique functions.
