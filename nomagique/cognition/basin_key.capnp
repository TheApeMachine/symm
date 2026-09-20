@0x97674075a0bd7e88;

using Go = import "/go.capnp";
$Go.package("cognition");
$Go.import("github.com/theapemachine/symm/nomagique/cognition");

interface BasinKey {
  write @0 (class :Data, contextBytes :Data) -> stream;
  done @1 () -> (out :Data);
}
