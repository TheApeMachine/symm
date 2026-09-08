# Recorded-market integration

`cmd.TestExecute` runs the production command composition against original
captured websocket payloads. The local peer routes recorded Level3 frames to
their subscribed connection and paces frames using `receivedAt`, preserving
capture order. The book reducer regenerates derived `l3_touch` witnesses;
those stored witnesses are counted separately and never injected as inputs.

The source directory contains contiguous, unmodified S3 capture `.jsonl`
objects beginning with capture sequence 1. `Tape.Read` validates the sequence,
run identity, and payload SHA-256 before delivery. Preserve the object's
zero-padded filename when caching it.

Run from the repository root:

```sh
GOFLAGS=-ldflags=-checklinkname=0 \
SYMM_REPLAY_CAPTURE_DIR=/absolute/path/to/captured-objects \
SYMM_REPLAY_THROUGH=2026-09-08T08:38:45Z \
SYMM_REPLAY_OUTPUT_DIR=/absolute/path/to/empty-replay-output \
go test -p 1 ./cmd -run '^TestExecute$' -count=1 -timeout=6m -v
```

`SYMM_REPLAY_THROUGH` is an optional end timestamp for a contiguous prefix.
The integration test has a five-minute startup/playback/drain budget. Use a
source interval that fits that budget. The output directory must be empty;
omitting it uses Go's temporary test directory.

The application uses its real normalizer, authentication, paper-account CLI,
transport callbacks, book reducers, configured signal/logic stages, learning
population, recording and dashboard transports. Existing Kraken credentials,
public REST access, the paper CLI, and the normal platform dependencies are
required. This test does not submit live orders.

Both the websocket learning feed and WebRTC diagnostics feed are observed.
Completion requires all transmitted originals to appear in the replay's
capture archive and the learning stage's completion counter to cover every
persisted ingress manifest. `replay-report.json` records the source run,
interval, input/output counts, configuration digest and build metadata.

This invocation starts with fresh learning state in an isolated archive.
It tests the recorded market workload through the current build; it does not
assert identical historical model state or random choices. Local startup and
socket backpressure can delay playback; the report retains maximum lateness.

The replay does not modify the original S3 capture or shared model checkpoint.

The same originals can benchmark decoding and integrity verification:

```sh
GOFLAGS=-ldflags=-checklinkname=0 \
SYMM_REPLAY_CAPTURE_DIR=/absolute/path/to/captured-objects \
SYMM_REPLAY_THROUGH=2026-09-08T08:38:45Z \
go test ./tests/replay -run '^$' -bench BenchmarkTapeRead -benchmem -benchtime=1x
```
