using Go = import "/go.capnp";
@0xe6913b7c084da2f5;
$Go.package("data");
$Go.import("github.com/theapemachine/symm/nomagique/data");

using import "../runtime/status.capnp".Status;

# A running fold publishes everything folded so far on every value, and keeps
# folding: a count, sum or mean of all that has arrived in its series. scope
# names that series; a new scope starts the fold again, so a count or sum
# never carries one series' values into the next. An unwired scope is one
# series. A fold that has nothing to publish this evaluation is idle: its
# consumers are not handed a zero standing in for a fold that did not report.
interface Reduce {
  write @0 (value :Float64, operator :Text, flush :Bool, running :Bool, scope :Text) -> stream;
  done @1 () -> Folded;
}

struct Folded {
  count  @0 :Int64;
  ready  @1 :Bool;
  status @2 :Status;
  union {
    idle @3 :Void;
    out  @4 :Float64;
  }
}
