# hindsight_export

Exports the decision record of a captured run as JSONL, so a run can be shared
and analyzed without moving the capture tape it came from (currently ~4GB).

The capture database holds the entire market record. What explains a run's
behaviour is much smaller: for each planning round, the gate it stopped at, the
council distribution it stopped on, and the search it ran if it got that far.
This reads exactly that.

## Usage

    go run ./cmd/hindsight_export ~/.symm/data/events.sqlite [flags]

    -run     run id (default: most recent run in the database)
    -out     output file (default: stdout)
    -symbol  only this symbol
    -status  only these predictiveStatus values (comma-separated)
    -acted   only rounds whose action is not "nothing"
    -limit   stop after N rounds (0 = no limit)
    -summary emit one aggregate object instead of per-round lines

Flags may appear before or after the database path.

## Where is the funnel dying?

    go run ./cmd/hindsight_export ~/.symm/data/events.sqlite -summary

`statusCounts` is the funnel: every round is counted against the gate it
stopped at. `roundsSearched` says how many reached the causal search at all.
`meanMoveMass` is the run-average council distribution — compare it against the
prior (stagnant 0.30, the four mid moves 0.15, the two extremes 0.05) to see
whether the advisors ever actually said anything.

## Sharing a specific moment

    go run ./cmd/hindsight_export ~/.symm/data/events.sqlite \
      -symbol HYPE/USD -out hype.jsonl

Each line carries `run`, `sequence` and `ordinal`, which address the exact
envelope in the capture, so any round in an export can be walked back to its
full market context in the Hindsight surface.

## Output

One JSON object per line — greps, streams, and loads into any dataframe without
a parser. Per round: identity, gate outcome (`predictiveStatus`, `reason`), the
full seven-move council distribution, the execution costs priced at that moment,
and the MCTS branch statistics when a search ran.

Note that `moveProbabilities` is the whole distribution, not the argmax. A 0.27
winner over a 0.26 runner-up is a different market read than a 0.9 winner, and
only the distribution shows that.
