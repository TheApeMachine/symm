@0xc412f533b6c9ae27;
using Go = import "/go.capnp";
$Go.package("controlflow");
$Go.import("github.com/theapemachine/symm/nomagique/controlflow");

# Enforces a graph-declared precondition before forwarding the supplied data.
# Unlike a filter, a false predicate is an explicit evaluation failure.
interface Require {
  write @0 (data :Data, test :Bool, reason :Text) -> stream;
  done @1 () -> (out :Data);
}
