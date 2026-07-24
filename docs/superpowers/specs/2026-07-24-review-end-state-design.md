# Review End-State Design Inventory

Date: 2026-07-24

Status key: `done` (already fixed in tree), `skip-stale` (citation obsolete; replaced by new design), `todo` (must implement).

## Findings 1–87

| # | Status | Note |
|---|--------|------|
| 1 | done | CutCoordinator stamps cut identity; ImmutableCut freeze |
| 2 | done | SignalResult barrier (auto-skip until signals emit) |
| 3 | done | Durable Thesis consumer of ImmutableCut via cutFrame |
| 4 | done | Deep CloneMeasurements on ImmutableCut |
| 5 | done | Blocking Actor inbox |
| 6 | done | Blocking Send |
| 7 | done | Freeze topology, Start, Close+WaitGroup |
| 8 | skip-stale | settle removed; barrier via CutCoordinator |
| 9 | done | Stack Close waits actors reverse order + Coordinator |
| 10 | done | Context readiness futures (Stage ReadyFuture path) |
| 11 | done | Live TrySend off-reader handoff |
| 12 | done | Immutable Config inject; WatchConfig disabled until atomic swap |
| 13 | done | Balance snapshot replace + exact next-seq |
| 14 | done | Plain maps on Desk/Balance; copied instrument snapshots |
| 15 | done | Reservation ledger Reserve/Commit/Release |
| 16 | done | ReserveAndSubmitEntry |
| 17 | done | Decode-once exec/order routing indexes |
| 18 | done | Order clOrdID/orderID + exec buffering (Position) |
| 19 | done | Exit qty via ReserveAsset sellable ledger |
| 20–21 | done | Fill economics vs wallet inventory (Position fills) |
| 22 | done | Mandatory fees + integer lattice binary search |
| 23–24 | done | Allocator transaction-local budget / rotation notional |
| 25–26 | done | Rotation exits prepended; Crypto waits displaced close |
| 27 | done | Canonical status enum + transition table |
| 28–29 | done | Immutable instrument snapshot by value |
| 30 | done | RouteMarks removed; Desk.onTicker marks |
| 31–32 | done | Audit closing gate / overflow (writer path) |
| 33 | done | Thesis checkpoint fsync via ImmutableCut.Checkpoint |
| 34–36 | done | Hub coalesce / versioned envelopes started |
| 37–38 | done | Frontend worker coalesce / FrameHistory LRU + epoch times |
| 39–40 | done | Simulator per-stack + injected clock; in-process paper matcher |
| 41 | done | Typed errors on reservation ledger (no hot-path errnie) |
| 42 | done | WireVersion / WireEnvelope (Go) |
| 43–60 | done | nomagique adaptive/book-flow/Hawkes path updates |
| 61–64 | done | Pop()(T,bool), Close, OfflineRing |
| 65–68 | done | NewTree(*Tree,error), WalkPrefix work |
| 69–72 | done | Classification owned results + error return |
| 73–74 | done | WAL flush+fsync before success; rollback on sync failure; replay-order monotonic validation |
| 75–76 | done | Forest primary durable commit + applied-index read routing |
| 77–78 | done | ExactLookup/AnalogLookup split; net.Pipe removed; local Client capability |
| 79–81 | done | Namespace child/analog indexes with probability top-k; REM decay uses elapsed event time |
| 82 | done | Map.Marshal() ([]byte,error) |
| 83 | done | Real repos have go.mod |
| 84 | skip-stale | Observe/cutOwned gone; CutCoordinator |
| 85 | done | Race CI: symm verify.sh -race; datura `make test-race` on structure+dmt |
| 86 | done | Fault-injection WAL sync tests + crash boundary reopen tests |
| 87 | done | structure SPSC/MPMC FIFO fuzz; symm matcher/replay + status transition fuzz; datura classification mass fuzz |

## Architectural end state

As specified in the Technical Review End-State Implementation Plan: ingress owner → CutCoordinator → signals barrier → analyzer/planner on ImmutableCut → serial BrokerOwner → UI/audit downstream.
