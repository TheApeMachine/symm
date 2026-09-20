@0x94e49f70f39ab4f0;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Classification {
  write @0 (class :Data, prob :Float64, support :UInt64) -> stream;
  done @1 () -> (
    winner :Data,
    runnerUp :Data,
    prob :Float64,
    contrast :Float64,
    support :UInt64,
    passed :Bool
  );
}
