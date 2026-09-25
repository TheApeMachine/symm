using Go = import "/go.capnp";
@0xbb7dcc1e398ea2b1;
$Go.package("store");
$Go.import("github.com/theapemachine/symm/nomagique/store");

using import "../data/measurement.capnp".Measurement;

# This is a recorded computational boundary, not a wall-clock bucket. Every
# publication is the completed result of one declared producer (signal OR
# logic module). An undefined coordinate is different from a missing producer.
struct MeasurementBoundary {
  version      @0 :UInt32;
  run          @1 :Text;
  sequence     @2 :Int64;
  previous     @3 :Int64;
  layout       @4 :Text;
  receipt      @5 :Data;
  publications @6 :List(Measurement);
}

# Boundary is the single owner of publication completeness, sequence lineage
# and coordinate coverage. Live writes seal publications. Replay writes the
# exact archived row; no signal is recalculated and no timestamp orders it.
# The persistence output is available even during measurement warm-up.
interface Boundary {
  write @0 (
    run          :Text,
    sequence     :Int64,
    receipt      :Data,
    layout       :Text,
    publications :List(Data),
    replay       :Data
  ) -> stream;
  done @1 () -> BoundaryResult;
}

struct BoundaryResult {
  row       @0 :Data;
  payload   @1 :Data;
  run       @2 :Text;
  sequence  @3 :Int64;
  receipt   @4 :Data;
  readiness @5 :Data;
  union {
    idle @6 :Void;
    warming @7 :Void;
    ready :group {
      values  @8 :List(Float64);
      present @9 :List(Bool);
    }
  }
}
