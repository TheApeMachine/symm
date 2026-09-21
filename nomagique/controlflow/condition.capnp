using Go = import "/go.capnp";
@0xc5094208a0b1c7d3;
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

using import "../runtime/status.capnp".Status;

interface Condition {
  write @0 (status :Status, test :Bool) -> stream;
  done @1 () -> (ready :Bool, busy :Bool, waiting :Bool, error :Bool, done :Bool);
}
