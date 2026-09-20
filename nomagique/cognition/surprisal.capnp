@0xb8410a3f0a76df28;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface Surprisal {
  write @0 (contextBytes :Data) -> stream;
  done @1 ();
}
