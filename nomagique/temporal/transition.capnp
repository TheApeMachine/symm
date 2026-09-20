@0x95a81b66f4ad792a;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

interface Transition {
  write @0 (a :Data) -> stream;
  done @1 () -> (out :Data);
}
