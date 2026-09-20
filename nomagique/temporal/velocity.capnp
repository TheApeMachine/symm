@0xe7d74680109a8b99;

using Go = import "/go.capnp";
$Go.package("temporal");
$Go.import("github.com/theapemachine/symm/nomagique/temporal");

interface Velocity {
  write @0 (val :Float64, ts :Float64) -> stream;
  done @1 ();
}
