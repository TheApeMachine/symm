@0xd07f87db81249766;

using Go = import "/go.capnp";
$Go.package("physics");
$Go.import("github.com/theapemachine/symm/nomagique/physics");

interface Simulation {
  write @0 (data :Data) -> stream;
  done @1 ();
}
