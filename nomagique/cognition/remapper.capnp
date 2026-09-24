@0xd0a4b6a3823dba94;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

using import "../store/radix.capnp".Retained;

interface Remapper extends(Retained) {
  write @0 (
    evidence    :Data,
    activations :List(Float64),
    observed    :List(Bool),
    authorities :List(Float64),
    ids         :List(Text),
    cursor      :Data,
    reset       :Bool
  ) -> stream;
  done @1 () -> (
    settled    :Bool,
    revision   :Int64,
    vocabulary :Text,
    regions    :Data,
    tokens     :List(Text),
    out        :Data
  );
}
