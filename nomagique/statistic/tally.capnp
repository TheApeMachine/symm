@0xbfea2b1fa4d2de84;
using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# An exact categorical tally. Counts is an explicit JSON object of UInt64
# multiplicities; {} is the empty multiset. Query does not add an observation.
interface Tally {
  write @0 (counts :Data, category :Text, query :Bool) -> stream;
  done @1 () -> (counts :Data, categories :List(Data), weights :List(UInt64), total :UInt64);
}
