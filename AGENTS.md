## Learned User Preferences

- Keep designs the user restores. Extend that code instead of replacing the subsystem.
- Change the code that was asked for, and report only issues that exist in the current tree.
- Use `io.Copy` between a `data.Value`, the store, and the hub. A prefix read or write is a Value seeded with that prefix, on a store client that stays safe to share across goroutines.
- Run multi-object downloads in detect and train. Drive progress from the listed object count, and fill the bar only when every object is done.
- Name Go tests `<file>_test.go` beside the source they cover.
- Size live panels to the available height, keep row order stable, and mark bottlenecks in that list.
- Make the audit measure the real calculations on a few active symbols and describe the result in plain language. Skip symbols with no movement, and check that each metric in SIGNALS.md actually moves.
- Fallbacks are forbidden. A missing case fails visibly.
- Sale price and return come from `broker/price.go` (`Price.Exit` and the book).

## Learned Workspace Facts

- The live path is an LMAX-style pipeline: signal handlers, then `strategy/training.go`. Each spot:trade tick is one `data.Value` whose layers hold the full tick state, written to S3 with `io.Copy`.
- The hub implements `io.Writer` and is passed into constructors that publish. It retains the latest frame and replays it when a dashboard connects.
- Object storage is SeaweedFS behind S3. Use the configured bucket. Offline commands run with `go run -overlay=overlay.json`.
- An aggregate tick has many symbols and no single symbol key. The grid is shared and each symbol has its own Sequitur grammar. SNM uses that metric's previous and current values, so one prior tick is enough. Region pins come from SIGNALS.md.
- Live tokens are the grid regions at or above the brightness midpoint. A matched enter or exit clears that symbol's token sequence. A sentence also ends when two regimes describe that symbol's metric energy in a shorter code than one regime. Policies are keys like `policy/current/R01/R05/R02/enter.bin` or `exit.bin`.
- Detect walks each symbol's tape as one continuous sequence. A tick is one moment; SeqIdx is the order Values are produced. Classes are up that clears friction, up that does not, chop, flat, and down. A, B, C, and D are distinct; a `c_tick` means an exit exists before C.
- Train reads committed `detections/` once, then handles each detection in its own goroutine: precursor A to B and holding B to C, each walked once on its own grid and Sequitur, with `grid.Observe` on those ticks and midpoint regions as tokens. Up that clears friction writes `enter.bin` and `exit.bin`. Down, chop, flat, and up that does not clear friction write no action. Cap chop and flat to the number of up detections.
- Hindsight shows the full per-symbol tape and draft A/B/C/D markers. A commit writes `detections/`, recalculates drafts from the tape held in memory, and learned detector parameters are saved for the next run. Marker edits do not list or re-download that span.
- Open lots live at `position/open/<symbol>.bin` and closed lives at `position/closed/<id>.bin`, both loaded at boot. A real winning close lowers that path's brightness bar. Train archives `policy/current` and leaves thresholds and open virtuals in place.
- The store client keeps an in-memory immutable radix of object keys. A permuted segment order is a view, and each terminal node holds the canonical key.
