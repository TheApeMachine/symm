## Learned User Preferences

- Keep designs the user restores. Extend that code instead of replacing the subsystem.
- Change the code that was asked for, and report only issues that exist in the current tree.
- Use `io.Copy` between a `data.Value`, the store, and the hub. A prefix read or write is a Value seeded with that prefix, on a store client that stays safe to share across goroutines.
- Run multi-object downloads in detect and train. Drive progress from the listed object count, and fill the bar only when every object is done.
- Name Go tests `<file>_test.go` beside the source they cover.
- Size live panels to the available height, keep row order stable, and mark bottlenecks in that list.
- Make the audit measure the real calculations and describe the result in plain language.

## Learned Workspace Facts

- The live path is an LMAX-style pipeline: signal handlers, then `strategy/training.go`. Each spot:trade tick is one `data.Value` whose layers hold the full tick state, written to S3 with `io.Copy`.
- The hub implements `io.Writer` and is passed into constructors that publish.
- Object storage is SeaweedFS behind S3. Use the configured bucket. Offline commands run with `go run -overlay=overlay.json`.
- An aggregate tick has many symbols and no single symbol key. The grid and grammar are global. SNM is computed from a metric's previous and current values.
- The predictive coder gates region tokens and feeds the dashboard predictive-coding view. Policies are keys like `policy/current/R01/R05/R02/enter.bin`, `exit.bin`, or `noop.bin`.
- Detect walks each symbol's tape as one continuous sequence. A tick is one moment; SeqIdx is the order Values are produced. Classes are up that clears friction, up that does not, chop, flat, and down. A, B, C, and D are distinct; a `c_tick` means an exit exists before C.
- Train lists detections once, then handles each detection in its own goroutine in the trainer: ticks from `a_tick` through `d_tick`, precursor A to B and holding B to D, `grid.Observe` on both, gate, Sequitur, then the two token files. Cap chop and flat to the number of up detections. Grid inputs come from the tick Value layers.
- Hindsight shows the full per-symbol tape and draft A/B/C/D markers. A commit writes `detections/`, recalculates drafts from the tape held in memory, and learned detector parameters are saved for the next run.
