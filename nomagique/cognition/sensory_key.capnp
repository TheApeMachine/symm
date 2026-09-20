@0x98b11e57e0d3a338;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface SensoryKey {
  write @0 (contextBytes :Data) -> stream;
  done @1 ();
}
