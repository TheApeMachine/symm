@0xa7a2f184cdaf6d7e;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# scope names the series being read. Readings under a new scope start a new
# history: a series never carries state into another one. An unwired scope is
# one series for the node's lifetime.
interface CausalVariance {
  write @0 (value :Float64, scope :Text) -> stream;
  done @1 () -> (out :Float64);
}
