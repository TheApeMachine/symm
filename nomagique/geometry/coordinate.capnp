@0xdd4ba30ddb381e2a;

using Go = import "/go.capnp";
$Go.package("geometry");
$Go.import("github.com/theapemachine/symm/nomagique/geometry");

interface Coordinate {
  write @0 (x :Int64, y :Int64) -> stream;
  done @1 ();
}
